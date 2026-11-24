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
#include <set>
#include <utility>
#include <memory>
#include <stdexcept>
#include <cereal/archives/portable_binary.hpp>
#include <cereal/types/string.hpp>
#include <cereal/types/vector.hpp>
#include <cereal/types/optional.hpp>
#include <cereal/types/variant.hpp>
#include <cereal/types/array.hpp>
#include <cereal/types/map.hpp>
#include <cereal/types/unordered_map.hpp>
#include <cereal/types/set.hpp>
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

struct SDItem {
    uint32_t itemID = 0; uint32_t seqID = 0; size_t count = 1;
    size_t duration[2] = {0,0};
    std::unordered_map<int, std::string> extAttrList;
    template<class Ar> void serialize(Ar & ar) { ar(itemID, seqID, count, duration[0], duration[1], extAttrList); }
};
struct SDDropItem {
    int x = 0; int y = 0; SDItem item;
    template<class Ar> void serialize(Ar & ar) { ar(x, y, item); }
};
struct SDDropItemList {
    int x = 0; int y = 0; std::vector<SDItem> itemList;
    template<class Ar> void serialize(Ar & ar) { ar(x, y, itemList); }
};
struct SDDropItemOdds {
    SDItem item; size_t odds = 1;
    template<class Ar> void serialize(Ar & ar) { ar(item, odds); }
};
struct SDDropOnDie {
    uint64_t playerUID = 0; bool allowDefaultDrop = false; std::vector<SDDropItemOdds> itemList;
    template<class Ar> void serialize(Ar & ar) { ar(playerUID, allowDefaultDrop, itemList); }
};
struct SDDropOnHit {
    std::vector<SDDropItemOdds> itemList;
    template<class Ar> void serialize(Ar & ar) { ar(itemList); }
};
struct SDBelt {
    std::array<SDItem, 6> list;
    template<class Ar> void serialize(Ar & ar) { ar(list); }
};
struct SDWear {
    std::unordered_map<int, SDItem> m_list;
    template<class Ar> void serialize(Ar & ar) { ar(m_list); }
};
struct SDShowSecuredItemList {
    std::vector<SDItem> itemList;
    template<class Ar> void serialize(Ar & ar) { ar(itemList); }
};
struct SDSellItem {
    SDItem item; std::vector<SDCostItem> costList;
    template<class Ar> void serialize(Ar & ar) { ar(item, costList); }
};
struct SDSellItemList {
    uint64_t npcUID = 0; std::vector<SDSellItem> list;
    template<class Ar> void serialize(Ar & ar) { ar(npcUID, list); }
};
struct SDAuctionItem {
    uint64_t auctionID = 0; SDChatPeer seller; std::string note; size_t timeLeft = 0; size_t price = 0; SDItem item;
    template<class Ar> void serialize(Ar & ar) { ar(auctionID, seller, note, timeLeft, price, item); }
};
struct SDAuctionItemList {
    int category = 0; std::vector<SDAuctionItem> itemList;
    template<class Ar> void serialize(Ar & ar) { ar(category, itemList); }
};
struct SDUpdateItem {
    SDItem item;
    template<class Ar> void serialize(Ar & ar) { ar(item); }
};
struct SDInventory {
    std::vector<SDItem> m_list;
    template<class Ar> void serialize(Ar & ar) { ar(m_list); }
};
struct SDGrantItemList {
    uint64_t playerUID = 0; std::vector<SDItem> itemList;
    template<class Ar> void serialize(Ar & ar) { ar(playerUID, itemList); }
};
struct SDItemGrant {
    size_t gold = 0; SDInventory inventory;
    template<class Ar> void serialize(Ar & ar) { ar(gold, inventory); }
};
struct SDDirectTradeItem {
    uint32_t itemID = 0; uint32_t seqID = 0; size_t count = 0;
    template<class Ar> void serialize(Ar & ar) { ar(itemID, seqID, count); }
};
struct SDDirectTradeOfferRequest {
    uint64_t uid = 0; size_t gold = 0; bool locked = false; std::vector<SDDirectTradeItem> itemList;
    template<class Ar> void serialize(Ar & ar) { ar(uid, gold, locked, itemList); }
};
struct SDDirectTradeOffer {
    uint64_t uid = 0; size_t gold = 0; bool locked = false; bool confirmed = false; std::vector<SDItem> itemList;
    template<class Ar> void serialize(Ar & ar) { ar(uid, gold, locked, confirmed, itemList); }
};
struct SDDirectTradeResult {
    uint64_t uid = 0; size_t gold = 0; SDInventory inventory;
    template<class Ar> void serialize(Ar & ar) { ar(uid, gold, inventory); }
};
struct SDEquipWear {
    uint64_t uid = 0; int wltype = 0; SDItem item;
    template<class Ar> void serialize(Ar & ar) { ar(uid, wltype, item); }
};
struct SDGrabWear {
    int wltype = 0; SDItem item;
    template<class Ar> void serialize(Ar & ar) { ar(wltype, item); }
};
struct SDEquipBelt {
    int slot = -1; SDItem item;
    template<class Ar> void serialize(Ar & ar) { ar(slot, item); }
};
struct SDGrabBelt {
    int slot = -1; SDItem item;
    template<class Ar> void serialize(Ar & ar) { ar(slot, item); }
};
struct SDBuyCost {
    SDItem item; std::vector<SDCostItem> costList;
    template<class Ar> void serialize(Ar & ar) { ar(item, costList); }
};
struct SDPickUpItemList {
    uint32_t failedItemID = 0; std::vector<SDItem> itemList;
    template<class Ar> void serialize(Ar & ar) { ar(failedItemID, itemList); }
};
struct SDStallItem {
    SDItem item; size_t price = 0;
    template<class Ar> void serialize(Ar & ar) { ar(item, price); }
};
struct SDStall {
    uint32_t dbid = 0; bool selling = false; std::string name; std::vector<SDStallItem> itemList;
    template<class Ar> void serialize(Ar & ar) { ar(dbid, selling, name, itemList); }
};
struct SDItemStorage {
    size_t gold = 0; SDWear wear; SDBelt belt; SDInventory inventory;
    template<class Ar> void serialize(Ar & ar) { ar(gold, wear, belt, inventory); }
};
struct SDWLDesp {
    SDWear wear; uint32_t hair = 0; uint32_t hairColor = 0;
    template<class Ar> void serialize(Ar & ar) { ar(wear, hair, hairColor); }
};
struct SDUIDWLDesp {
    uint64_t uid = 0; SDWLDesp desp;
    template<class Ar> void serialize(Ar & ar) { ar(uid, desp); }
};
struct SDStartGameScene {
    uint64_t uid = 0; uint64_t mapUID = 0; int x = -1; int y = -1; int direction = 0; SDWLDesp desp; std::string name; uint32_t nameColor = 0; int viewRange = 15;
    template<class Ar> void serialize(Ar & ar) { ar(uid, mapUID, x, y, direction, desp, name, nameColor, viewRange); }
};

struct SDRuntimeConfig {
    std::unordered_map<std::string, std::string> m_config;
    template<class Ar> void serialize(Ar & ar) { ar(m_config); }
};
struct SDPlayerConfig {
    SDMagicKeyList magicKeyList; SDRuntimeConfig runtimeConfig;
    template<class Ar> void serialize(Ar & ar) { ar(magicKeyList, runtimeConfig); }
};
struct SDBuffedAbility {
    SDTaggedValMap dc0, dc1, mc0, mc1, sc0, sc1, ac0, ac1, mac0, mac1;
    SDTaggedValMap dcHit, mcHit, dcDodge, mcDodge, speed, luckCurse;
    template<class Ar> void serialize(Ar & ar) { ar(dc0,dc1,mc0,mc1,sc0,sc1,ac0,ac1,mac0,mac1,dcHit,mcHit,dcDodge,mcDodge,speed,luckCurse); }
};
struct SDHealth {
    uint64_t uid = 0; int hp = 0; int mp = 0; int maxHP = 0; int maxMP = 0; int hpRecover = 0; int mpRecover = 0;
    SDTaggedValMap buffedMaxHP, buffedMaxMP, buffedHPRecover, buffedMPRecover;
    template<class Ar> void serialize(Ar & ar) { ar(uid,hp,mp,maxHP,maxMP,hpRecover,mpRecover,buffedMaxHP,buffedMaxMP,buffedHPRecover,buffedMPRecover); }
};
struct SDBuffIDList {
    uint64_t uid = 0; std::vector<uint32_t> idList;
    template<class Ar> void serialize(Ar & ar) { ar(uid,idList); }
};
struct SDRegisterQuest {
    std::string name;
    template<class Ar> void serialize(Ar & ar) { ar(name); }
};
struct SDQueryQuestUID {
    std::string name;
    template<class Ar> void serialize(Ar & ar) { ar(name); }
};
struct SDTeamJobReq {
    int minLevel = -1; int maxLevel = -1; int minCapacity = 0; std::set<uint32_t> magicIDList;
    template<class Ar> void serialize(Ar & ar) { ar(minLevel,maxLevel,minCapacity,magicIDList); }
};
struct SDTeamReq {
    int maxMember = 0; bool autoAccept = false; std::array<std::optional<SDTeamJobReq>,3> jobReqList;
    template<class Ar> void serialize(Ar & ar) { ar(maxMember,autoAccept,jobReqList); }
};
struct SDTeamPlayer {
    uint64_t uid = 0; uint32_t level = 0; int job = 0; std::string name;
    template<class Ar> void serialize(Ar & ar) { ar(uid,level,job,name); }
};
struct SDTeamCandidate {
    SDTeamPlayer player;
    template<class Ar> void serialize(Ar & ar) { ar(player); }
};
struct SDRequestJoinTeam {
    SDTeamPlayer player; int job = 0; int stat = 0; std::vector<uint32_t> magicIDList;
    template<class Ar> void serialize(Ar & ar) { ar(player,job,stat,magicIDList); }
};
struct SDTeamMemberList {
    uint64_t teamLeader = 0; std::vector<SDTeamPlayer> memberList; SDTeamReq req;
    template<class Ar> void serialize(Ar & ar) { ar(teamLeader,memberList,req); }
};
struct SDTransferTeamLeader {
    std::vector<uint64_t> memberList; SDTeamReq req;
    template<class Ar> void serialize(Ar & ar) { ar(memberList,req); }
};
struct SDTeamInfo {
    uint64_t leader = 0; std::string name; int job = 0; int level = 0; int memberCount = 0; SDTeamReq req;
    template<class Ar> void serialize(Ar & ar) { ar(leader,name,job,level,memberCount,req); }
};

struct LUAVar;
struct LUAWrapper {
    std::shared_ptr<LUAVar> value;
};
struct LUAEntry {
    LUAWrapper key;
    LUAWrapper value;
};
struct LUAVar {
    int32_t kind = 0;
    uint8_t nilPlaceholder = 0;
    std::vector<LUAWrapper> array;
    std::vector<LUAEntry> table;
    int64_t integer = 0;
    double number = 0;
    bool boolean = false;
    std::string str;
};

template<class Ar> void saveLuaVar(Ar & ar, const LUAVar &);
template<class Ar> void saveLuaWrapper(Ar & ar, const LUAWrapper & wrapper)
{
    uint8_t valid = wrapper.value ? 1 : 0;
    ar(valid);
    if(valid){ saveLuaVar(ar, *wrapper.value); }
}
template<class Ar> void saveLuaVar(Ar & ar, const LUAVar & value)
{
    ar(value.kind);
    switch(value.kind){
        case 0: ar(value.nilPlaceholder); break;
        case 1:
            {
                uint64_t size = value.array.size();
                ar(size);
                for(const auto &item: value.array){ saveLuaWrapper(ar, item); }
                break;
            }
        case 2:
            {
                uint64_t size = value.table.size();
                ar(size);
                for(const auto &item: value.table){
                    saveLuaWrapper(ar, item.key);
                    saveLuaWrapper(ar, item.value);
                }
                break;
            }
        case 3: ar(value.integer); break;
        case 4: ar(value.number); break;
        case 5: ar(value.boolean); break;
        case 6: ar(value.str); break;
        default: throw std::runtime_error("bad lua kind");
    }
}

static LUAWrapper wrapLua(const LUAVar &value)
{
    LUAWrapper wrapper;
    wrapper.value = std::make_shared<LUAVar>(value);
    return wrapper;
}
static LUAVar luaInteger(int64_t value){ LUAVar v; v.kind=3; v.integer=value; return v; }
static LUAVar luaNumber(double value){ LUAVar v; v.kind=4; v.number=value; return v; }
static LUAVar luaBool(bool value){ LUAVar v; v.kind=5; v.boolean=value; return v; }
static LUAVar luaString(const std::string &value){ LUAVar v; v.kind=6; v.str=value; return v; }
static LUAVar luaNilVar(){ LUAVar v; v.kind=0; return v; }

struct LuaVarFixture {
    LUAVar value;
    template<class Ar> void serialize(Ar & ar){ saveLuaVar(ar, value); }
};
struct SDRemoteCallX {
    std::string code; LUAVar args;
    template<class Ar> void serialize(Ar & ar){ ar(code); saveLuaVar(ar, args); }
};
struct SDRemoteCallResultX {
    std::vector<std::string> error; std::vector<LUAVar> varList;
    template<class Ar> void serialize(Ar & ar){
        ar(error);
        uint64_t size = varList.size(); ar(size);
        for(const auto &item: varList){ saveLuaVar(ar, item); }
    }
};
struct SDSendNotifyX {
    uint64_t key = 0; uint64_t seqID = 0; LUAVar varList; bool waitConsume = false;
    template<class Ar> void serialize(Ar & ar){ ar(key, seqID); saveLuaVar(ar, varList); ar(waitConsume); }
};

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
    SDItem item; item.itemID=100; item.seqID=7; item.count=3; item.duration[0]=90; item.duration[1]=100; item.extAttrList={{1,"abc"}}; dump("SDItem", item);
    SDItem item2; item2.itemID=101; item2.seqID=8; item2.count=1; dump("SDItemEmptyExtAttrs", item2);
    SDDropItem drop; drop.x=1; drop.y=2; drop.item=item; dump("SDDropItem", drop);
    SDDropItemList dropList; dropList.x=3; dropList.y=4; dropList.itemList={item,item2}; dump("SDDropItemList", dropList);
    SDDropItemOdds dropOdds; dropOdds.item=item; dropOdds.odds=2; dump("SDDropItemOdds", dropOdds);
    SDDropOnDie dropDie; dropDie.playerUID=0x1122334455667788ULL; dropDie.allowDefaultDrop=true; dropDie.itemList={dropOdds}; dump("SDDropOnDie", dropDie);
    SDDropOnHit dropHit; dropHit.itemList={dropOdds}; dump("SDDropOnHit", dropHit);
    SDBelt belt; belt.list={item,{},{},{},{},{}}; dump("SDBelt", belt);
    SDWear wear; wear.m_list={{1,item}}; dump("SDWear", wear);
    SDShowSecuredItemList secured; secured.itemList={item,item2}; dump("SDShowSecuredItemList", secured);
    SDSellItem sellItem; sellItem.item=item; sellItem.costList={cost}; dump("SDSellItem", sellItem);
    SDSellItemList sellList; sellList.npcUID=0x1122334455667788ULL; sellList.list={sellItem}; dump("SDSellItemList", sellList);
    SDAuctionItem auctionItem; auctionItem.auctionID=0x0102030405060708ULL; auctionItem.seller=peer; auctionItem.note="note"; auctionItem.timeLeft=3600; auctionItem.price=500; auctionItem.item=item; dump("SDAuctionItem", auctionItem);
    SDAuctionItemList auctionList; auctionList.category=2; auctionList.itemList={auctionItem}; dump("SDAuctionItemList", auctionList);
    SDUpdateItem updateItem; updateItem.item=item; dump("SDUpdateItem", updateItem);
    SDInventory inventory; inventory.m_list={item,item2}; dump("SDInventory", inventory);
    SDGrantItemList grantList; grantList.playerUID=0x1122334455667788ULL; grantList.itemList={item,item2}; dump("SDGrantItemList", grantList);
    SDItemGrant itemGrant; itemGrant.gold=1000; itemGrant.inventory=inventory; dump("SDItemGrant", itemGrant);
    SDDirectTradeItem directItem; directItem.itemID=100; directItem.seqID=7; directItem.count=2; dump("SDDirectTradeItem", directItem);
    SDDirectTradeOfferRequest tradeRequest; tradeRequest.uid=0x1122334455667788ULL; tradeRequest.gold=100; tradeRequest.locked=true; tradeRequest.itemList={directItem}; dump("SDDirectTradeOfferRequest", tradeRequest);
    SDDirectTradeOffer tradeOffer; tradeOffer.uid=0x1122334455667788ULL; tradeOffer.gold=200; tradeOffer.locked=true; tradeOffer.confirmed=true; tradeOffer.itemList={item}; dump("SDDirectTradeOffer", tradeOffer);
    SDDirectTradeResult tradeResult; tradeResult.uid=0x1122334455667788ULL; tradeResult.gold=300; tradeResult.inventory=inventory; dump("SDDirectTradeResult", tradeResult);
    SDEquipWear equipWear; equipWear.uid=42; equipWear.wltype=3; equipWear.item=item; dump("SDEquipWear", equipWear);
    SDGrabWear grabWear; grabWear.wltype=3; grabWear.item=item; dump("SDGrabWear", grabWear);
    SDEquipBelt equipBelt; equipBelt.slot=2; equipBelt.item=item; dump("SDEquipBelt", equipBelt);
    SDGrabBelt grabBelt; grabBelt.slot=2; grabBelt.item=item; dump("SDGrabBelt", grabBelt);
    SDBuyCost buyCost; buyCost.item=item; buyCost.costList={cost}; dump("SDBuyCost", buyCost);
    SDPickUpItemList pickUp; pickUp.failedItemID=99; pickUp.itemList={item,item2}; dump("SDPickUpItemList", pickUp);
    SDStallItem stallItem; stallItem.item=item; stallItem.price=500; dump("SDStallItem", stallItem);
    SDStall stall; stall.dbid=7; stall.selling=true; stall.name="shop"; stall.itemList={stallItem}; dump("SDStall", stall);
    SDItemStorage storage; storage.gold=1000; storage.wear=wear; storage.belt=belt; storage.inventory=inventory; dump("SDItemStorage", storage);
    SDWLDesp wlDesp; wlDesp.wear=wear; wlDesp.hair=3; wlDesp.hairColor=4; dump("SDWLDesp", wlDesp);
    SDUIDWLDesp uidWL; uidWL.uid=42; uidWL.desp=wlDesp; dump("SDUIDWLDesp", uidWL);
    SDStartGameScene startScene; startScene.uid=42; startScene.mapUID=0x1122334455667788ULL; startScene.x=-10; startScene.y=20; startScene.direction=3; startScene.desp=wlDesp; startScene.name="hero"; startScene.nameColor=0x11223344; startScene.viewRange=15; dump("SDStartGameScene", startScene);
    SDRuntimeConfig runtime; runtime.m_config={{"volume","0.5"}}; dump("SDRuntimeConfig", runtime);
    SDPlayerConfig playerConfig; playerConfig.magicKeyList.keyList={{1,'a'}}; playerConfig.runtimeConfig=runtime; dump("SDPlayerConfig", playerConfig);
    SDBuffedAbility buff; buff.dc0.valMap={{1,10}}; buff.mc0.valMap={{2,20}}; buff.dcHit.valMap={{3,30}}; buff.speed.valMap={{1,5}}; dump("SDBuffedAbility", buff);
    SDHealth health; health.uid=42; health.hp=100; health.mp=50; health.maxHP=200; health.maxMP=100; health.hpRecover=3; health.mpRecover=4; health.buffedMaxHP.valMap={{1,20}}; health.buffedMaxMP.valMap={{1,10}}; health.buffedHPRecover.valMap={{1,2}}; health.buffedMPRecover.valMap={{1,1}}; dump("SDHealth", health);
    SDBuffIDList buffIDs; buffIDs.uid=42; buffIDs.idList={1,2,3}; dump("SDBuffIDList", buffIDs);
    SDRegisterQuest registerQuest; registerQuest.name="quest"; dump("SDRegisterQuest", registerQuest);
    SDQueryQuestUID queryQuest; queryQuest.name="quest"; dump("SDQueryQuestUID", queryQuest);
    SDTeamJobReq jobReq; jobReq.minLevel=10; jobReq.maxLevel=30; jobReq.minCapacity=5; jobReq.magicIDList={2,5}; dump("SDTeamJobReq", jobReq);
    SDTeamReq teamReq; teamReq.maxMember=5; teamReq.autoAccept=true; teamReq.jobReqList={jobReq,std::nullopt,std::nullopt}; dump("SDTeamReq", teamReq);
    SDTeamPlayer teamPlayer; teamPlayer.uid=42; teamPlayer.level=20; teamPlayer.job=1; teamPlayer.name="hero"; dump("SDTeamPlayer", teamPlayer);
    SDTeamCandidate candidate; candidate.player=teamPlayer; dump("SDTeamCandidate", candidate);
    SDRequestJoinTeam joinReq; joinReq.player=teamPlayer; joinReq.job=1; joinReq.stat=10; joinReq.magicIDList={2,5}; dump("SDRequestJoinTeam", joinReq);
    SDTeamMemberList teamMembers; teamMembers.teamLeader=42; teamMembers.memberList={teamPlayer,{43,21,2,"bob"}}; teamMembers.req=teamReq; dump("SDTeamMemberList", teamMembers);
    SDTransferTeamLeader transfer; transfer.memberList={42,43}; transfer.req=teamReq; dump("SDTransferTeamLeader", transfer);
    SDTeamInfo teamInfo; teamInfo.leader=42; teamInfo.name="party"; teamInfo.job=1; teamInfo.level=20; teamInfo.memberCount=2; teamInfo.req=teamReq; dump("SDTeamInfo", teamInfo);
    LUAVar luaTable; luaTable.kind=2; luaTable.table.push_back({wrapLua(luaString("k")), wrapLua(luaInteger(7))});
    LUAVar nestedLua; nestedLua.kind=1; nestedLua.array={wrapLua(luaInteger(42)), wrapLua(luaString("x")), wrapLua(luaBool(true)), LUAWrapper{}, wrapLua(luaTable)};
    LuaVarFixture luaFixture; luaFixture.value=nestedLua; dump("LuaVarNested", luaFixture);
    SDRemoteCallX remoteCall; remoteCall.code="return 1"; remoteCall.args=nestedLua; dump("SDRemoteCall", remoteCall);
    SDRemoteCallResultX remoteResult; remoteResult.error={"e1","e2"}; remoteResult.varList={luaInteger(5),luaString("ok")}; dump("SDRemoteCallResult", remoteResult);
    SDSendNotifyX sendNotify; sendNotify.key=0x1122334455667788ULL; sendNotify.seqID=0x0102030405060708ULL; sendNotify.varList=nestedLua; sendNotify.waitConsume=true; dump("SDSendNotify", sendNotify);
}
