# mir2x 协议对齐表（客户端复刻基线）

> 本文件由 `tools/gen_protocol_doc.py` 从 mir2x C++ 头文件**抽取生成**，不是手抄。
> 基线提交：mir2x `5826493`；源码根：`E:\Desktop\game\mir2x`（只读参照，禁止修改）。
> 本轮只服务 **client-godot 客户端分支**；服务端不在本分支内实现。

## 0. 校验状态

| 项 | 结果 |
|---|---|
| CM 枚举条数（有效） | 87 |
| SM 枚举条数（有效） | 115 |
| CM 注册表缺失 | 0 |
| SM 注册表缺失 | 0 |
| 字节序 | 小端（PortableBinary 固定） |
| 结构对齐 | `#pragma pack(1)` |

## 1. 传输层帧格式

### 1.1 整数编码

| 项 | 规则 | 依据 |
|---|---|---|
| 帧内整数 | **小端** | `cereal::PortableBinaryOutputArchive` |
| 变长长度 | **LEB128**（低 7 位，高位续读） | `msgf::encodeLength` |
| 比特域 | 按 C++ 位域低位在前 | `clientmsg.hpp:109` / `servermsg.hpp:130` |

### 1.2 帧结构

```
[headCode(1B)] [respID:LEB128 ?] [bodySize:LEB128 ?] [mask:maskLen B ?] [body:bodySize B ?]
```

- `headCode & 0x80` = hasResp，表示带应答 ID。
- 消息体形态查表决定，四类：

| type | 线上结构 | 说明 |
|---|---|---|
| 0 | 仅 headCode | 空体 |
| 1 | `respID? + dataCount(LEB128) + mask + body` | 定长 XOR 压缩 |
| 2 | `respID? + body(dataLen)` | 定长直传 |
| 3 | `respID? + bodySize(LEB128) + body` | 变长，多为 cerealf 块 |

### 1.3 XOR 压缩（type 1）

- `maskLen = (dataLen + 7) / 8`
- `dataLen >= 256` 时切 64 位宽：`maskLen = (dataLen + 63) / 64`，按 8 字节整块判零
- mask 第 i 位 = 原始第 i 字节是否非零；body 顺序存非零字节

> **坑点**：解码必须用消息表里的 `dataLen`（原长），不能用收到的 `dataCount`，否则尾部丢失。

### 1.4 cerealf 封装（type 3 负载）

序列化结果尾部追加 1 字节压缩标记：

| 尾字节 | 含义 |
|---|---|
| 0 | CF_NONE，未压缩 |
| 1 | CF_ZSTD，zstd 压缩 |
| 2 | CF_XOR，前 1 字节为原始长度 |

编码细节：64 位平台 `size_t`/`vector` 计数为 **u64**；`std::string` = `u64 长度 + 原始字节`。

### 1.5 关键定长结构体（实测 sizeof，ucrt64 g++ -std=c++20）

| 结构体 | 字节 | 备注 |
|---|---|---|
| `CMPing` / `SMPing` | 4 | u32 tick |
| `CMLogin` | 136 | 2 × StaticBuffer<64> |
| `SMAction` / `CMAction` | 44 | u64 + u64 + ActionNode(28) |
| `ActionNode` | 28 | 位域 3 + 4×i16 + u64 union + 4B extParam |
| `SMOnlineOK` | 177 | 见定义 |
| `SMCORecord` | 49 | u64 + u64 + ActionNode + 12B union |
| `SMCastMagic` | 36 | 见定义 |

**`StaticBuffer<N>` = 2 + (N+1) 字节**：`u16 size` + `data[N+1]`。
`size` 是实际字节数，读字符串要按 `size` 截断。

## 2. 客户端 → 服务端（CM）

| code | 名称 | 类型 | 定长 |
|---:|---|---|---|
| `0` | `CM_NONE_0` | 0 — 空体（无负载） | — |
| `1` | `CM_PING` | 2 — 定长 + 不压缩 | sizeof(CMPing |
| `2` | `CM_LOGIN` | 1 — 定长 + XOR 压缩 | sizeof(CMLogin |
| `3` | `CM_QUERYCHAR` | 0 — 空体（无负载） | — |
| `4` | `CM_CREATECHAR` | 1 — 定长 + XOR 压缩 | sizeof(CMCreateChar |
| `5` | `CM_DELETECHAR` | 1 — 定长 + XOR 压缩 | sizeof(CMDeleteChar |
| `6` | `CM_CHANGEPASSWORD` | 1 — 定长 + XOR 压缩 | sizeof(CMChangePassword |
| `7` | `CM_ONLINE` | 0 — 空体（无负载） | — |
| `8` | `CM_ACTION` | 1 — 定长 + XOR 压缩 | sizeof(CMAction |
| `9` | `CM_SETMAGICKEY` | 1 — 定长 + XOR 压缩 | sizeof(CMSetMagicKey |
| `10` | `CM_SETRUNTIMECONFIG` | 1 — 定长 + XOR 压缩 | sizeof(CMSetRuntimeConfig |
| `11` | `CM_QUERYCORECORD` | 1 — 定长 + XOR 压缩 | sizeof(CMQueryCORecord |
| `12` | `CM_QUERYNPCNAME` | 1 — 定长 + XOR 压缩 | sizeof(CMQueryNPCName |
| `13` | `CM_REQUESTADDHEALTH` | 1 — 定长 + XOR 压缩 | sizeof(CMRequestAddHealth |
| `14` | `CM_REQUESTADDEXP` | 1 — 定长 + XOR 压缩 | sizeof(CMRequestAddExp |
| `15` | `CM_REQUESTDIE` | 0 — 空体（无负载） | — |
| `16` | `CM_REQUESTREVIVE` | 0 — 空体（无负载） | — |
| `17` | `CM_REQUESTLOGOUT` | 0 — 空体（无负载） | — |
| `18` | `CM_REQUESTHOSTOFFLINE` | 0 — 空体（无负载） | — |
| `19` | `CM_REQUESTKILLPETS` | 0 — 空体（无负载） | — |
| `20` | `CM_REQUESTRETRIEVESECUREDITEM` | 1 — 定长 + XOR 压缩 | sizeof(CMRequestRetrieveSecuredItem |
| `21` | `CM_REQUESTSPACEMOVE` | 1 — 定长 + XOR 压缩 | sizeof(CMRequestSpaceMove |
| `22` | `CM_REQUESTMAGICDAMAGE` | 1 — 定长 + XOR 压缩 | sizeof(CMRequestMagicDamage |
| `23` | `CM_PICKUP` | 1 — 定长 + XOR 压缩 | sizeof(CMPickUp |
| `24` | `CM_QUERYBASEMAPUID` | 1 — 定长 + XOR 压缩 | sizeof(CMQueryBaseMapUID |
| `25` | `CM_QUERYGOLD` | 0 — 空体（无负载） | — |
| `26` | `CM_QUERYUIDBUFF` | 1 — 定长 + XOR 压缩 | sizeof(CMQueryUIDBuff |
| `27` | `CM_QUERYPLAYERNAME` | 1 — 定长 + XOR 压缩 | sizeof(CMQueryPlayerName |
| `28` | `CM_QUERYPLAYERWLDESP` | 1 — 定长 + XOR 压缩 | sizeof(CMQueryPlayerWLDesp |
| `29` | `CM_QUERYCHATPEERLIST` | 1 — 定长 + XOR 压缩 | sizeof(CMQueryChatPeerList |
| `30` | `CM_QUERYCHATMESSAGE` | 1 — 定长 + XOR 压缩 | sizeof(CMQueryChatMessage |
| `31` | `CM_CREATECHATGROUP` | 1 — 定长 + XOR 压缩 | sizeof(CMCreateChatGroup |
| `32` | `CM_CREATEACCOUNT` | 1 — 定长 + XOR 压缩 | sizeof(CMCreateAccount |
| `33` | `CM_NPCEVENT` | 1 — 定长 + XOR 压缩 | sizeof(CMNPCEvent |
| `34` | `CM_QUERYSELLITEMLIST` | 1 — 定长 + XOR 压缩 | sizeof(CMQuerySellItemList |
| `35` | `CM_DROPITEM` | 1 — 定长 + XOR 压缩 | sizeof(CMDropItem |
| `36` | `CM_CONSUMEITEM` | 1 — 定长 + XOR 压缩 | sizeof(CMConsumeItem |
| `37` | `CM_MAKEITEM` | 1 — 定长 + XOR 压缩 | sizeof(CMMakeItem |
| `38` | `CM_BUY` | 1 — 定长 + XOR 压缩 | sizeof(CMBuy |
| `39` | `CM_ADDFRIEND` | 1 — 定长 + XOR 压缩 | sizeof(CMAddFriend |
| `40` | `CM_ACCEPTADDFRIEND` | 1 — 定长 + XOR 压缩 | sizeof(CMAcceptAddFriend |
| `41` | `CM_REJECTADDFRIEND` | 1 — 定长 + XOR 压缩 | sizeof(CMRejectAddFriend |
| `42` | `CM_BLOCKPLAYER` | 1 — 定长 + XOR 压缩 | sizeof(CMBlockPlayer |
| `43` | `CM_CHATMESSAGE` | 3 — 变长 + 不压缩 | — |
| `44` | `CM_PLAYERSAY` | 1 — 定长 + XOR 压缩 | sizeof(CMPlayerSay |
| `45` | `CM_PLAYERBROADCAST` | 1 — 定长 + XOR 压缩 | sizeof(CMPlayerBroadcast |
| `46` | `CM_REQUESTEQUIPWEAR` | 1 — 定长 + XOR 压缩 | sizeof(CMRequestEquipWear |
| `47` | `CM_REQUESTGRABWEAR` | 1 — 定长 + XOR 压缩 | sizeof(CMRequestGrabWear |
| `48` | `CM_REQUESTEQUIPBELT` | 1 — 定长 + XOR 压缩 | sizeof(CMRequestEquipBelt |
| `49` | `CM_REQUESTGRABBELT` | 1 — 定长 + XOR 压缩 | sizeof(CMRequestGrabBelt |
| `50` | `CM_REQUESTCONSUMEBELT` | 1 — 定长 + XOR 压缩 | sizeof(CMRequestConsumeBelt |
| `51` | `CM_REQUESTJOINTEAM` | 1 — 定长 + XOR 压缩 | sizeof(CMRequestJoinTeam |
| `52` | `CM_REQUESTLEAVETEAM` | 1 — 定长 + XOR 压缩 | sizeof(CMRequestLeaveTeam |
| `53` | `CM_CREATETEAM` | 3 — 变长 + 不压缩 | — |
| `54` | `CM_SETTEAMREQUIREMENT` | 3 — 变长 + 不压缩 | — |
| `55` | `CM_QUERYTEAMINFO` | 1 — 定长 + XOR 压缩 | sizeof(CMQueryTeamInfo |
| `56` | `CM_TRANSFERTEAMLEADER` | 1 — 定长 + XOR 压缩 | sizeof(CMTransferTeamLeader |
| `57` | `CM_SETFOLLOWTEAMLEADER` | 1 — 定长 + XOR 压缩 | sizeof(CMSetFollowTeamLeader |
| `58` | `CM_REQUESTLATESTCHATMESSAGE` | 1 — 定长 + XOR 压缩 | sizeof(CMRequestLatestChatMessage |
| `59` | `CM_QUERYRANKING` | 2 — 定长 + 不压缩 | sizeof(CMQueryRanking |
| `60` | `CM_CLAIMDELIVERY` | 1 — 定长 + XOR 压缩 | sizeof(CMClaimDelivery |
| `61` | `CM_QUERYAUCTIONITEMLIST` | 2 — 定长 + 不压缩 | sizeof(CMQueryAuctionItemList |
| `62` | `CM_REGISTERAUCTIONITEM` | 1 — 定长 + XOR 压缩 | sizeof(CMRegisterAuctionItem |
| `63` | `CM_BUYAUCTIONITEM` | 1 — 定长 + XOR 压缩 | sizeof(CMBuyAuctionItem |
| `64` | `CM_UNREGISTERAUCTIONITEM` | 1 — 定长 + XOR 压缩 | sizeof(CMUnregisterAuctionItem |
| `65` | `CM_REQUESTDIRECTTRADE` | 1 — 定长 + XOR 压缩 | sizeof(CMRequestDirectTrade |
| `66` | `CM_RESPONDDIRECTTRADE` | 1 — 定长 + XOR 压缩 | sizeof(CMRespondDirectTrade |
| `67` | `CM_UPDATEDIRECTTRADE` | 3 — 变长 + 不压缩 | — |
| `68` | `CM_COMMITDIRECTTRADE` | 1 — 定长 + XOR 压缩 | sizeof(CMCommitDirectTrade |
| `69` | `CM_CANCELDIRECTTRADE` | 1 — 定长 + XOR 压缩 | sizeof(CMCancelDirectTrade |
| `70` | `CM_ADDSTALLITEM` | 1 — 定长 + XOR 压缩 | sizeof(CMAddStallItem |
| `71` | `CM_REMOVESTALLITEM` | 1 — 定长 + XOR 压缩 | sizeof(CMRemoveStallItem |
| `72` | `CM_STARTSTALL` | 1 — 定长 + XOR 压缩 | sizeof(CMStartStall |
| `73` | `CM_CLOSESTALL` | 0 — 空体（无负载） | — |
| `74` | `CM_QUERYSTALL` | 1 — 定长 + XOR 压缩 | sizeof(CMQueryStall |
| `75` | `CM_BUYSTALLITEM` | 1 — 定长 + XOR 压缩 | sizeof(CMBuyStallItem |
| `76` | `CM_QUERYPLAYERGUILD` | 1 — 定长 + XOR 压缩 | sizeof(CMQueryPlayerGuild |
| `77` | `CM_INVITEGUILDMEMBER` | 1 — 定长 + XOR 压缩 | sizeof(CMInviteGuildMember |
| `78` | `CM_RESPONDGUILDINVITE` | 1 — 定长 + XOR 压缩 | sizeof(CMRespondGuildInvite |
| `79` | `CM_LEAVEGUILD` | 0 — 空体（无负载） | — |
| `80` | `CM_KICKGUILDMEMBER` | 1 — 定长 + XOR 压缩 | sizeof(CMKickGuildMember |
| `81` | `CM_TRANSFERGUILDLEADER` | 1 — 定长 + XOR 压缩 | sizeof(CMTransferGuildLeader |
| `82` | `CM_DISBANDGUILD` | 0 — 空体（无负载） | — |
| `83` | `CM_QUERYGUILDINFO` | 0 — 空体（无负载） | — |
| `84` | `CM_SETGUILDNOTICE` | 1 — 定长 + XOR 压缩 | sizeof(CMSetGuildNotice |
| `85` | `CM_SETGUILDMEMBERTITLE` | 1 — 定长 + XOR 压缩 | sizeof(CMSetGuildMemberTitle |
| `86` | `CM_GUILDCHAT` | 1 — 定长 + XOR 压缩 | sizeof(CMGuildChat |

## 3. 服务端 → 客户端（SM）

| code | 名称 | 类型 | 定长 |
|---:|---|---|---|
| `0` | `SM_NONE_0` | 0 — 空体（无负载） | — |
| `1` | `SM_OK` | 0 — 空体（无负载） | — |
| `2` | `SM_ERROR` | 0 — 空体（无负载） | — |
| `3` | `SM_PING` | 2 — 定长 + 不压缩 | sizeof(SMPing |
| `4` | `SM_UID` | 2 — 定长 + 不压缩 | sizeof(SMUID |
| `5` | `SM_LOGINOK` | 0 — 空体（无负载） | — |
| `6` | `SM_LOGINERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMLoginError |
| `7` | `SM_CREATEACCOUNTOK` | 0 — 空体（无负载） | — |
| `8` | `SM_CREATEACCOUNTERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMCreateAccountError |
| `9` | `SM_CHANGEPASSWORDOK` | 0 — 空体（无负载） | — |
| `10` | `SM_CHANGEPASSWORDERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMChangePasswordError |
| `11` | `SM_QUERYCHAROK` | 1 — 定长 + XOR 压缩 | sizeof(SMQueryCharOK |
| `12` | `SM_QUERYCHARERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMQueryCharError |
| `13` | `SM_CREATECHAROK` | 0 — 空体（无负载） | — |
| `14` | `SM_CREATECHARERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMCreateCharError |
| `15` | `SM_CREATECHATGROUP` | 3 — 变长 + 不压缩 | — |
| `16` | `SM_ADDFRIENDACCEPTED` | 3 — 变长 + 不压缩 | — |
| `17` | `SM_ADDFRIENDREJECTED` | 3 — 变长 + 不压缩 | — |
| `18` | `SM_DELETECHAROK` | 0 — 空体（无负载） | — |
| `19` | `SM_DELETECHARERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMDeleteCharError |
| `20` | `SM_ONLINEOK` | 1 — 定长 + XOR 压缩 | sizeof(SMOnlineOK |
| `21` | `SM_LOGOUTOK` | 0 — 空体（无负载） | — |
| `22` | `SM_ONLINEERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMOnlineError |
| `23` | `SM_STARTGAMESCENE` | 3 — 变长 + 不压缩 | — |
| `24` | `SM_PLAYERCONFIG` | 3 — 变长 + 不压缩 | — |
| `25` | `SM_FRIENDLIST` | 3 — 变长 + 不压缩 | — |
| `26` | `SM_LEARNEDMAGICLIST` | 3 — 变长 + 不压缩 | — |
| `27` | `SM_PLAYERWLDESP` | 3 — 变长 + 不压缩 | — |
| `28` | `SM_ACTION` | 1 — 定长 + XOR 压缩 | sizeof(SMAction |
| `29` | `SM_CORECORD` | 1 — 定长 + XOR 压缩 | sizeof(SMCORecord |
| `30` | `SM_HEALTH` | 3 — 变长 + 不压缩 | — |
| `31` | `SM_NEXTSTRIKE` | 0 — 空体（无负载） | — |
| `32` | `SM_NOTIFYDEAD` | 1 — 定长 + XOR 压缩 | sizeof(SMNotifyDead |
| `33` | `SM_DEADFADEOUT` | 1 — 定长 + XOR 压缩 | sizeof(SMDeadFadeOut |
| `34` | `SM_EXP` | 1 — 定长 + XOR 压缩 | sizeof(SMExp |
| `35` | `SM_BUFF` | 1 — 定长 + XOR 压缩 | sizeof(SMBuff |
| `36` | `SM_BUFFIDLIST` | 3 — 变长 + 不压缩 | — |
| `37` | `SM_MISS` | 1 — 定长 + XOR 压缩 | sizeof(SMMiss |
| `38` | `SM_CASTMAGIC` | 1 — 定长 + XOR 压缩 | sizeof(SMCastMagic |
| `39` | `SM_OFFLINE` | 1 — 定长 + XOR 压缩 | sizeof(SMOffline |
| `40` | `SM_PICKUPERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMPickUpError |
| `41` | `SM_REMOVEGROUNDITEM` | 1 — 定长 + XOR 压缩 | sizeof(SMRemoveGroundItem |
| `42` | `SM_NPCXMLLAYOUT` | 3 — 变长 + 不压缩 | — |
| `43` | `SM_NPCSELL` | 3 — 变长 + 不压缩 | — |
| `44` | `SM_NPCERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMNPCError |
| `45` | `SM_STARTINVOP` | 3 — 变长 + 不压缩 | — |
| `46` | `SM_STARTINPUT` | 3 — 变长 + 不压缩 | — |
| `47` | `SM_GOLD` | 1 — 定长 + XOR 压缩 | sizeof(SMGold |
| `48` | `SM_INVOPCOST` | 1 — 定长 + XOR 压缩 | sizeof(SMInvOpCost |
| `49` | `SM_STRIKEGRID` | 1 — 定长 + XOR 压缩 | sizeof(SMStrikeGrid |
| `50` | `SM_SELLITEMLIST` | 3 — 变长 + 不压缩 | — |
| `51` | `SM_TEXT` | 3 — 变长 + 不压缩 | — |
| `52` | `SM_PARTEXT` | 3 — 变长 + 不压缩 | — |
| `53` | `SM_PLAYERNAME` | 3 — 变长 + 不压缩 | — |
| `54` | `SM_BUILDVERSION` | 1 — 定长 + XOR 压缩 | sizeof(SMBuildVersion |
| `55` | `SM_INVENTORY` | 3 — 变长 + 不压缩 | — |
| `56` | `SM_BELT` | 3 — 变长 + 不压缩 | — |
| `57` | `SM_UPDATEITEM` | 3 — 变长 + 不压缩 | — |
| `58` | `SM_REMOVEITEM` | 1 — 定长 + XOR 压缩 | sizeof(SMRemoveItem |
| `59` | `SM_REMOVESECUREDITEM` | 1 — 定长 + XOR 压缩 | sizeof(SMRemoveSecuredItem |
| `60` | `SM_BUYSUCCEED` | 1 — 定长 + XOR 压缩 | sizeof(SMBuySucceed |
| `61` | `SM_BUYERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMBuyError |
| `62` | `SM_GROUNDITEMIDLIST` | 3 — 变长 + 不压缩 | — |
| `63` | `SM_GROUNDFIREWALLLIST` | 3 — 变长 + 不压缩 | — |
| `64` | `SM_SAFEZONELIST` | 3 — 变长 + 不压缩 | — |
| `65` | `SM_EQUIPWEAR` | 3 — 变长 + 不压缩 | — |
| `66` | `SM_EQUIPWEARERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMEquipWearError |
| `67` | `SM_GRABWEAR` | 3 — 变长 + 不压缩 | — |
| `68` | `SM_GRABWEARERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMGrabWearError |
| `69` | `SM_EQUIPBELT` | 3 — 变长 + 不压缩 | — |
| `70` | `SM_EQUIPBELTERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMEquipBeltError |
| `71` | `SM_GRABBELT` | 3 — 变长 + 不压缩 | — |
| `72` | `SM_GRABBELTERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMGrabBeltError |
| `73` | `SM_WEARITEMDURATION` | 1 — 定长 + XOR 压缩 | sizeof(SMWearItemDuration |
| `74` | `SM_SHOWSECUREDITEMLIST` | 3 — 变长 + 不压缩 | — |
| `75` | `SM_PLAYERTEAMLEADER` | 1 — 定长 + XOR 压缩 | sizeof(SMPlayerTeamLeader |
| `76` | `SM_TEAMMEMBERLIST` | 3 — 变长 + 不压缩 | — |
| `77` | `SM_TEAMCANDIDATE` | 3 — 变长 + 不压缩 | — |
| `78` | `SM_TEAMERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMTeamError |
| `79` | `SM_TEAMINFO` | 3 — 变长 + 不压缩 | — |
| `80` | `SM_QUESTDESPUPDATE` | 3 — 变长 + 不压缩 | — |
| `81` | `SM_QUESTDESPLIST` | 3 — 变长 + 不压缩 | — |
| `82` | `SM_CHATMESSAGELIST` | 3 — 变长 + 不压缩 | — |
| `83` | `SM_PLAYERSAY` | 1 — 定长 + XOR 压缩 | sizeof(SMPlayerSay |
| `84` | `SM_PLAYERBROADCAST` | 1 — 定长 + XOR 压缩 | sizeof(SMPlayerBroadcast |
| `85` | `SM_RANKINGLIST` | 3 — 变长 + 不压缩 | — |
| `86` | `SM_CLAIMDELIVERYERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMClaimDeliveryError |
| `87` | `SM_AUCTIONITEMLIST` | 3 — 变长 + 不压缩 | — |
| `88` | `SM_AUCTIONREGISTERERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMAuctionRegisterError |
| `89` | `SM_AUCTIONBUYERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMAuctionBuyError |
| `90` | `SM_AUCTIONUNREGISTERERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMAuctionUnregisterError |
| `91` | `SM_QUERYCHATPEERLISTOK` | 3 — 变长 + 不压缩 | — |
| `92` | `SM_QUERYCHATMESSAGEOK` | 3 — 变长 + 不压缩 | — |
| `93` | `SM_CHATMESSAGEOK` | 3 — 变长 + 不压缩 | — |
| `94` | `SM_ADDFRIENDOK` | 3 — 变长 + 不压缩 | — |
| `95` | `SM_CREATECHATGROUPOK` | 3 — 变长 + 不压缩 | — |
| `96` | `SM_REQUESTDIRECTTRADE` | 1 — 定长 + XOR 压缩 | sizeof(SMRequestDirectTrade |
| `97` | `SM_STARTDIRECTTRADE` | 1 — 定长 + XOR 压缩 | sizeof(SMStartDirectTrade |
| `98` | `SM_UPDATEDIRECTTRADE` | 3 — 变长 + 不压缩 | — |
| `99` | `SM_COMPLETEDIRECTTRADE` | 1 — 定长 + XOR 压缩 | sizeof(SMCompleteDirectTrade |
| `100` | `SM_CLOSEDIRECTTRADE` | 1 — 定长 + XOR 压缩 | sizeof(SMCloseDirectTrade |
| `101` | `SM_DIRECTTRADEERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMDirectTradeError |
| `102` | `SM_CODISAPPEAR` | 1 — 定长 + XOR 压缩 | sizeof(SMCODisappear |
| `103` | `SM_REVIVE` | 1 — 定长 + XOR 压缩 | sizeof(SMRevive |
| `104` | `SM_VIEWRANGE` | 1 — 定长 + XOR 压缩 | sizeof(SMViewRange |
| `105` | `SM_NPCNAME` | 3 — 变长 + 不压缩 | — |
| `106` | `SM_STALL` | 3 — 变长 + 不压缩 | — |
| `107` | `SM_STALLERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMStallError |
| `108` | `SM_PLAYERSTALL` | 1 — 定长 + XOR 压缩 | sizeof(SMPlayerStall |
| `109` | `SM_QUERYSTALLOK` | 3 — 变长 + 不压缩 | — |
| `110` | `SM_PLAYERGUILD` | 3 — 变长 + 不压缩 | — |
| `111` | `SM_GUILDERROR` | 1 — 定长 + XOR 压缩 | sizeof(SMGuildError |
| `112` | `SM_GUILDINFO` | 3 — 变长 + 不压缩 | — |
| `113` | `SM_GUILDCHAT` | 1 — 定长 + XOR 压缩 | sizeof(SMGuildChat |
| `114` | `SM_GUILDWARLIST` | 3 — 变长 + 不压缩 | — |

## 4. 客户端复刻优先级分组

### P0 —— 走通「登录 → 进图 → 移动 → 攻击 → 聊天」必需

| 方向 | 消息 | 用途 |
|---|---|---|
| C→S | `CM_PING` / `CM_LOGIN` / `CM_QUERYCHAR` / `CM_ONLINE` | 握手、登录、选人 |
| C→S | `CM_ACTION` | **移动/站立/攻击全部走这一条** |
| C→S | `CM_PLAYERSAY` / `CM_CHATMESSAGE` | 聊天 |
| S→C | `SM_PING` / `SM_LOGINOK` / `SM_LOGINERROR` | 握手应答 |
| S→C | `SM_QUERYCHAROK` / `SM_ONLINEOK` / `SM_STARTGAMESCENE` | 进图三件套 |
| S→C | `SM_ACTION` / `SM_CORECORD` / `SM_CODISAPPEAR` / `SM_OFFLINE` | 视野内实体同步 |
| S→C | `SM_HEALTH` / `SM_EXP` / `SM_GOLD` / `SM_INVENTORY` / `SM_BELT` | 状态面板 |
| S→C | `SM_PLAYERSAY` / `SM_CHATMESSAGELIST` / `SM_TEXT` | 聊天与公告 |

### P1 —— 背包与穿戴

`CM_PICKUP`、`CM_DROPITEM`、`CM_CONSUMEITEM`、`CM_REQUESTEQUIPWEAR/GRABWEAR`、
`CM_REQUESTEQUIPBELT/GRABBELT/CONSUMEBELT`、`CM_BUY` 及其 `SM_*` 应答与错误码。

### P2 —— 后续扩展

组队、行会、交易、摆摊、拍卖、好友、排行榜、任务、魔法。

## 5. 关键枚举（客户端必须先实现）

| 枚举 | 取值 | 用途 |
|---|---|---|
| `dir8_t` | 1=上, 2=右上, 3=右, 4=右下, 5=下, 6=左下, 7=左, 8=左上 | 8 方向 |
| `ActionType` | 1=SPAWN … 15=MINE | 动作类型 |
| `UIDType` | 1=COR, 2=MAP, 3=NPC, 4=MON, 5=PLY | uid 高 4 位 |
| `JobType` | 1=战士, 2=道士, 4=法师（位掩码） | 职业 |

**UID 位布局**（`uidf.cpp`）：

```
63      : 保留位
62..59  : type（4 bit）
58..35  : id（24 bit）
34..31  : peerIndex（4 bit）
30..0   : seq
```
