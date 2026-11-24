#include <cstdint>
#include <cstdio>
#include <sstream>
#include <string>
#include <vector>
#include <optional>
#include <variant>
#include <array>
#include <map>
#include <unordered_map>
#include <utility>
#include <cereal/archives/portable_binary.hpp>
#include <cereal/types/string.hpp>
#include <cereal/types/vector.hpp>
#include <cereal/types/optional.hpp>
#include <cereal/types/variant.hpp>
#include <cereal/types/array.hpp>
#include <cereal/types/map.hpp>
#include <cereal/types/unordered_map.hpp>
#include <cereal/types/utility.hpp>

struct SDSendPackage {
    uint8_t type = 0;
    std::vector<uint8_t> buf;
    template<class Ar> void serialize(Ar & ar) { ar(type, buf); }
};
struct SDObserverNetPackage {
    uint64_t uid = 0; int x = -1; int y = -1; SDSendPackage package;
    template<class Ar> void serialize(Ar & ar) { ar(uid, x, y, package); }
};
struct SDInitQuest {
    uint32_t questID = 0; std::string fullScriptName;
    template<class Ar> void serialize(Ar & ar) { ar(questID, fullScriptName); }
};
struct SDInitGuard {
    uint32_t monsterID = 0; uint64_t mapUID = 0; int x = 0; int y = 0; bool strictLoc = false; int direction = 0;
    template<class Ar> void serialize(Ar & ar) { ar(monsterID, mapUID, x, y, strictLoc, direction); }
};
struct SDInitPlayer {
    uint32_t dbid = 0; uint32_t channID = 0; std::string name; uint64_t mapUID = 0; int x = 0; int y = 0; bool strictLoc = false;
    int hp = 0; int mp = 0; size_t exp = 0; size_t gold = 0; bool gender = true; int job = 0;
    int hair = 0; int hairColor = 0; int pkPoint = 0; int viewRange = 15;
    template<class Ar> void serialize(Ar & ar) { ar(dbid, channID, name, mapUID, x, y, strictLoc, hp, mp, exp, gold, gender, job, hair, hairColor, pkPoint, viewRange); }
};
struct SDInitNPChar {
    uint16_t lookID = 0; std::string npcName; std::string fullScriptName; uint64_t mapUID = 0; int x = 0; int y = 0; bool strictLoc = false; int gfxDir = 0;
    template<class Ar> void serialize(Ar & ar) { ar(lookID, npcName, fullScriptName, mapUID, x, y, strictLoc, gfxDir); }
};
struct SDInitMonster {
    uint32_t monsterID = 0; uint64_t mapUID = 0; int x = 0; int y = 0; bool strictLoc = false; int direction = 0; uint64_t masterUID = 0;
    template<class Ar> void serialize(Ar & ar) { ar(monsterID, mapUID, x, y, strictLoc, direction, masterUID); }
};
struct SDNPCXMLLayout {
    uint64_t npcUID = 0; std::string eventPath; std::string xmlLayout;
    template<class Ar> void serialize(Ar & ar) { ar(npcUID, eventPath, xmlLayout); }
};
struct SDChatGroupMember {
    uint32_t dbid = 0; uint16_t priority = 0;
    template<class Ar> void serialize(Ar & ar) { ar(dbid, priority); }
};
struct SDChatPeerPlayerVar {
    bool gender = false; int job = 0;
    template<class Ar> void serialize(Ar & ar) { ar(gender, job); }
};
struct SDChatPeerGroupVar {
    uint32_t creator = 0; uint64_t createtime = 0; std::vector<SDChatGroupMember> memberList;
    template<class Ar> void serialize(Ar & ar) { ar(creator, createtime, memberList); }
};
struct SDChatPeer {
    uint32_t id = 0; std::string name; std::optional<uint64_t> avatar;
    std::variant<std::monostate, SDChatPeerPlayerVar, SDChatPeerGroupVar> despvar;
    template<class Ar> void serialize(Ar & ar) { ar(id, name, avatar, despvar); }
};
struct SDChatPeerID {
    uint64_t m_data = 0;
    template<class Ar> void serialize(Ar & ar) { ar(m_data); }
};

struct SDStartInvOp {
    int invOp = 0; uint64_t uid = 0; std::string queryTag; std::string commitTag; std::vector<std::u8string> typeList;
    template<class Ar> void serialize(Ar & ar) { ar(invOp, uid, queryTag, commitTag, typeList); }
};
struct SDStartInput {
    uint64_t uid = 0; std::string title; std::string commitTag; bool security = false;
    template<class Ar> void serialize(Ar & ar) { ar(uid, title, commitTag, security); }
};
struct SDNPCSell {
    uint64_t npcUID = 0; std::vector<uint32_t> itemList;
    template<class Ar> void serialize(Ar & ar) { ar(npcUID, itemList); }
};
struct SDCostItem {
    uint32_t itemID = 0; size_t count = 0;
    template<class Ar> void serialize(Ar & ar) { ar(itemID, count); }
};
struct SDPlayerName {
    uint64_t uid = 0; std::string name; uint32_t nameColor = 0;
    template<class Ar> void serialize(Ar & ar) { ar(uid, name, nameColor); }
};
struct SDNPCName {
    uint64_t uid = 0; std::string name;
    template<class Ar> void serialize(Ar & ar) { ar(uid, name); }
};
struct SDPlayerGuild {
    uint32_t dbid = 0; uint32_t guildID = 0; std::string guildName; std::string title;
    template<class Ar> void serialize(Ar & ar) { ar(dbid, guildID, guildName, title); }
};
struct SDGuildWar {
    uint32_t guildID = 0; std::string guildName; uint64_t endTime = 0;
    template<class Ar> void serialize(Ar & ar) { ar(guildID, guildName, endTime); }
};
struct SDGuildWarList {
    uint32_t guildID = 0; std::vector<SDGuildWar> warList;
    template<class Ar> void serialize(Ar & ar) { ar(guildID, warList); }
};
struct SDGuildMember {
    uint32_t dbid = 0; std::string name; int rank = 0; std::string title; bool online = false;
    template<class Ar> void serialize(Ar & ar) { ar(dbid, name, rank, title, online); }
};
struct SDGuildInfo {
    uint32_t guildID = 0; std::string name; std::string notice; std::vector<SDGuildMember> memberList;
    template<class Ar> void serialize(Ar & ar) { ar(guildID, name, notice, memberList); }
};
struct SDGroundItemIDList {
    struct GridItemIDList {
        int x = -1; int y = -1; std::vector<uint32_t> itemIDList;
        template<class Ar> void serialize(Ar & ar) { ar(x, y, itemIDList); }
    };
    uint64_t mapUID = 0; std::vector<GridItemIDList> gridItemIDList;
    template<class Ar> void serialize(Ar & ar) { ar(mapUID, gridItemIDList); }
};
struct SDGridFireWall {
    int x = 0; int y = 0; int count = 0;
    template<class Ar> void serialize(Ar & ar) { ar(x, y, count); }
};
struct SDGroundFireWallList {
    uint64_t mapUID = 0; std::vector<SDGridFireWall> fireWallList;
    template<class Ar> void serialize(Ar & ar) { ar(mapUID, fireWallList); }
};
struct SDSafeZone {
    std::vector<std::array<int, 4>> rectList; std::vector<std::array<int, 3>> circleList;
    template<class Ar> void serialize(Ar & ar) { ar(rectList, circleList); }
};
struct SDSafeZoneList {
    uint64_t mapUID = 0; SDSafeZone zone;
    template<class Ar> void serialize(Ar & ar) { ar(mapUID, zone); }
};
struct SDLearnedMagic {
    uint32_t magicID = 0; int exp = 0;
    template<class Ar> void serialize(Ar & ar) { ar(magicID, exp); }
};
struct SDLearnedMagicList {
    std::vector<SDLearnedMagic> magicList;
    template<class Ar> void serialize(Ar & ar) { ar(magicList); }
};
struct SDMagicKeyList {
    std::unordered_map<uint32_t, char> keyList;
    template<class Ar> void serialize(Ar & ar) { ar(keyList); }
};
struct SDAddFriendNotif {
    int notif = 0;
    template<class Ar> void serialize(Ar & ar) { ar(notif); }
};
struct SDBlockPlayerNotif {
    int notif = 0;
    template<class Ar> void serialize(Ar & ar) { ar(notif); }
};
struct SDChatMessageDBSeq {
    uint64_t id = 0; uint64_t timestamp = 0;
    template<class Ar> void serialize(Ar & ar) { ar(id, timestamp); }
};
struct SDChatMessage {
    std::optional<SDChatMessageDBSeq> seq; std::optional<uint64_t> refer; SDChatPeerID from; SDChatPeerID to; std::string message;
    template<class Ar> void serialize(Ar & ar) { ar(seq, refer, from, to, message); }
};
struct SDNPCEvent {
    int x = 0; int y = 0; uint64_t mapUID = 0; std::string path; std::string event; std::optional<std::string> value;
    template<class Ar> void serialize(Ar & ar) { ar(x, y, mapUID, path, event, value); }
};
struct SDTaggedValMap {
    std::map<int, int> valMap;
    template<class Ar> void serialize(Ar & ar) { ar(valMap); }
};
struct SDPeerConfig {
    double dropRate = 1.0; double goldRate = 1.0;
    template<class Ar> void serialize(Ar & ar) { ar(dropRate, goldRate); }
};
struct SDSysSlavePeerPort {
    int port = 0;
    template<class Ar> void serialize(Ar & ar) { ar(port); }
};
struct SDSysSlavePeerList {
    std::map<size_t, std::pair<std::string, uint32_t>> list;
    template<class Ar> void serialize(Ar & ar) { ar(list); }
};
struct SDSysPeerIndex {
    size_t index = 0; std::string masterConfig;
    template<class Ar> void serialize(Ar & ar) { ar(index, masterConfig); }
};
struct SDQuestDespUpdate {
    std::string name; std::string fsm; std::optional<std::string> desp;
    template<class Ar> void serialize(Ar & ar) { ar(name, fsm, desp); }
};
struct SDRankingEntry {
    uint32_t dbid = 0; uint32_t level = 0; uint64_t gold = 0; std::string name;
    template<class Ar> void serialize(Ar & ar) { ar(dbid, level, gold, name); }
};
struct SDRankingList {
    uint8_t type = 0; std::vector<SDRankingEntry> entries;
    template<class Ar> void serialize(Ar & ar) { ar(type, entries); }
};
using SDChatPeerList = std::vector<SDChatPeer>;
using SDUIDList = std::vector<uint64_t>;

template<class T> void dump(const char *name, const T & t) {
    std::ostringstream ss(std::ios::binary);
    { cereal::PortableBinaryOutputArchive ar(ss); ar(t); }
    auto s=ss.str();
    std::printf("%s\t%zu\t", name, s.size());
    for(unsigned char c : s) std::printf("%02x", c);
    std::printf("\n");
}

int main(){
    SDSendPackage send; send.type=0x12; send.buf={0,1,2,255}; dump("SDSendPackage", send);
    SDObserverNetPackage observer; observer.uid=0x1122334455667788ULL; observer.x=-2; observer.y=300; observer.package=send; dump("SDObserverNetPackage", observer);
    SDInitQuest quest; quest.questID=100; quest.fullScriptName="quest/abc.lua"; dump("SDInitQuest", quest);
    SDInitGuard guard; guard.monsterID=0x11223344; guard.mapUID=0x5566778899aabbccULL; guard.x=-2; guard.y=3; guard.strictLoc=true; guard.direction=5; dump("SDInitGuard", guard);
    SDInitPlayer player; player.dbid=7; player.channID=9; player.name="hero"; player.mapUID=0x1122334455667788ULL; player.x=-10; player.y=20; player.strictLoc=true; player.hp=100; player.mp=50; player.exp=123456789; player.gold=987654321; player.gender=true; player.job=2; player.hair=3; player.hairColor=4; player.pkPoint=5; player.viewRange=15; dump("SDInitPlayer", player);
    SDInitNPChar npc; npc.lookID=0x1234; npc.npcName="merchant"; npc.fullScriptName="npc/shop.lua"; npc.mapUID=0x0102030405060708ULL; npc.x=11; npc.y=22; npc.strictLoc=false; npc.gfxDir=7; dump("SDInitNPChar", npc);
    SDInitMonster monster; monster.monsterID=0xaabbccdd; monster.mapUID=0x0102030405060708ULL; monster.x=-3; monster.y=4; monster.strictLoc=true; monster.direction=6; monster.masterUID=0x1122334455667788ULL; dump("SDInitMonster", monster);
    SDNPCXMLLayout layout; layout.npcUID=42; layout.eventPath="npc/test"; layout.xmlLayout="<xml/>"; dump("SDNPCXMLLayout", layout);
    SDChatGroupMember chatMember1; chatMember1.dbid=1; chatMember1.priority=2; dump("SDChatGroupMember", chatMember1);
    SDChatPeerPlayerVar playerVar; playerVar.gender=true; playerVar.job=2; dump("SDChatPeerPlayerVar", playerVar);
    SDChatPeerGroupVar groupVar; groupVar.creator=7; groupVar.createtime=0x1122334455667788ULL; groupVar.memberList={chatMember1,{5,9}}; dump("SDChatPeerGroupVar", groupVar);
    SDChatPeer peer; peer.id=42; peer.name="alice"; peer.avatar=123456789; peer.despvar=groupVar; dump("SDChatPeer", peer);
    SDChatPeerID peerID; peerID.m_data=(uint64_t(3)<<32)|42; dump("SDChatPeerID", peerID);
    SDStartInvOp inv; inv.invOp=5; inv.uid=0x1122334455667788ULL; inv.queryTag="q"; inv.commitTag="c"; inv.typeList={u8"item",u8"gold"}; dump("SDStartInvOp", inv);
    SDStartInput input; input.uid=42; input.title="rename"; input.commitTag="ok"; input.security=true; dump("SDStartInput", input);
    SDNPCSell sell; sell.npcUID=0x1122334455667788ULL; sell.itemList={1,2,0xffffffffU}; dump("SDNPCSell", sell);
    SDCostItem cost; cost.itemID=55; cost.count=3; dump("SDCostItem", cost);
    SDPlayerName playerName; playerName.uid=42; playerName.name="hero"; playerName.nameColor=0x11223344; dump("SDPlayerName", playerName);
    SDNPCName npcName; npcName.uid=43; npcName.name="merchant"; dump("SDNPCName", npcName);
    SDPlayerGuild playerGuild; playerGuild.dbid=7; playerGuild.guildID=9; playerGuild.guildName="guild"; playerGuild.title="leader"; dump("SDPlayerGuild", playerGuild);
    SDGuildWar war1; war1.guildID=9; war1.guildName="enemy"; war1.endTime=123456789; dump("SDGuildWar", war1);
    SDGuildWarList warList; warList.guildID=7; warList.warList={war1,{10,"other",987654321}}; dump("SDGuildWarList", warList);
    SDGuildMember member1; member1.dbid=1; member1.name="alice"; member1.rank=1; member1.title="leader"; member1.online=true; dump("SDGuildMember", member1);
    SDGuildInfo guildInfo; guildInfo.guildID=7; guildInfo.name="guild"; guildInfo.notice="hello"; guildInfo.memberList={member1,{2,"bob",2,"member",false}}; dump("SDGuildInfo", guildInfo);
    SDGroundItemIDList ground; ground.mapUID=0x1122334455667788ULL; ground.gridItemIDList={{1,2,{10,20}},{-3,4,{}}}; dump("SDGroundItemIDList", ground);
    SDGridFireWall wall; wall.x=1; wall.y=2; wall.count=3; dump("SDGridFireWall", wall);
    SDGroundFireWallList walls; walls.mapUID=0x1122334455667788ULL; walls.fireWallList={wall,{4,5,6}}; dump("SDGroundFireWallList", walls);
    SDSafeZone zone; zone.rectList={{1,2,3,4},{5,6,7,8}}; zone.circleList={{9,10,11}}; dump("SDSafeZone", zone);
    SDSafeZoneList zoneList; zoneList.mapUID=0x1122334455667788ULL; zoneList.zone=zone; dump("SDSafeZoneList", zoneList);
    SDLearnedMagic magic; magic.magicID=1; magic.exp=100; dump("SDLearnedMagic", magic);
    SDLearnedMagicList magicList; magicList.magicList={magic,{2,200}}; dump("SDLearnedMagicList", magicList);
    SDMagicKeyList magicKeys; magicKeys.keyList={{1,'a'}}; dump("SDMagicKeyList", magicKeys);
    SDAddFriendNotif addFriend; addFriend.notif=3; dump("SDAddFriendNotif", addFriend);
    SDBlockPlayerNotif blockPlayer; blockPlayer.notif=4; dump("SDBlockPlayerNotif", blockPlayer);
    SDChatMessageDBSeq dbSeq; dbSeq.id=0x1122334455667788ULL; dbSeq.timestamp=123456789; dump("SDChatMessageDBSeq", dbSeq);
    SDChatMessage chat; chat.seq=dbSeq; chat.refer=0x0102030405060708ULL; chat.from.m_data=(uint64_t(2)<<32)|42; chat.to.m_data=(uint64_t(3)<<32)|43; chat.message="hi"; dump("SDChatMessage", chat);
    SDNPCEvent npcEvent; npcEvent.x=1; npcEvent.y=2; npcEvent.mapUID=0x1122334455667788ULL; npcEvent.path="npc/p"; npcEvent.event="ev"; npcEvent.value=std::string("val"); dump("SDNPCEvent", npcEvent);
    SDTaggedValMap tagged; tagged.valMap={{1,10},{2,20}}; dump("SDTaggedValMap", tagged);
    SDPeerConfig peerConfig; peerConfig.dropRate=1.5; peerConfig.goldRate=0.75; dump("SDPeerConfig", peerConfig);
    SDSysSlavePeerPort peerPort; peerPort.port=7000; dump("SDSysSlavePeerPort", peerPort);
    SDSysSlavePeerList peerList; peerList.list={{1,{"host1",7001}},{2,{"host2",7002}}}; dump("SDSysSlavePeerList", peerList);
    SDSysPeerIndex peerIndex; peerIndex.index=3; peerIndex.masterConfig="cfg"; dump("SDSysPeerIndex", peerIndex);
    SDQuestDespUpdate questDesp; questDesp.name="quest"; questDesp.fsm="main"; questDesp.desp=std::string("desp"); dump("SDQuestDespUpdate", questDesp);
    SDRankingEntry rankingEntry; rankingEntry.dbid=1; rankingEntry.level=50; rankingEntry.gold=123456789; rankingEntry.name="hero"; dump("SDRankingEntry", rankingEntry);
    SDRankingList ranking; ranking.type=1; ranking.entries={rankingEntry,{2,40,987654321,"bob"}}; dump("SDRankingList", ranking);
    SDChatPeerList peerListValue{peer,{43,"bob",std::nullopt,SDChatPeerPlayerVar{false,1}}}; dump("SDChatPeerList", peerListValue);
    SDUIDList uidList{1,0x1122334455667788ULL}; dump("SDUIDList", uidList);
}
