#include <cstdint>
#include <cstdio>
#include <sstream>
#include <string>
#include <vector>
#include <optional>
#include <variant>
#include <cereal/archives/portable_binary.hpp>
#include <cereal/types/string.hpp>
#include <cereal/types/vector.hpp>
#include <cereal/types/optional.hpp>
#include <cereal/types/variant.hpp>

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
    SDChatGroupMember member1; member1.dbid=1; member1.priority=2; dump("SDChatGroupMember", member1);
    SDChatPeerPlayerVar playerVar; playerVar.gender=true; playerVar.job=2; dump("SDChatPeerPlayerVar", playerVar);
    SDChatPeerGroupVar groupVar; groupVar.creator=7; groupVar.createtime=0x1122334455667788ULL; groupVar.memberList={member1,{5,9}}; dump("SDChatPeerGroupVar", groupVar);
    SDChatPeer peer; peer.id=42; peer.name="alice"; peer.avatar=123456789; peer.despvar=groupVar; dump("SDChatPeer", peer);
    SDChatPeerID peerID; peerID.m_data=(uint64_t(3)<<32)|42; dump("SDChatPeerID", peerID);
}