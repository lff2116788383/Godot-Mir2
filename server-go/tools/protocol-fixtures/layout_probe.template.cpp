#include <cstdint>
#include <cstddef>
#include <cstdio>
#include <cstring>

template<size_t Capacity> struct StaticBuffer { uint16_t size; uint8_t data[Capacity + 1]; };
template<typename T, size_t Capacity> struct StaticVector { uint16_t size; T data[Capacity]; };

/*__CONSTANTS__*/

enum ActionType : int { /*__ACTION_ENUM__*/ };
struct ActionSpawn; struct ActionStand; struct ActionMove; struct ActionJump; struct ActionPushMove; struct ActionSpaceMove; struct ActionAttack; struct ActionPickUp; struct ActionSpell; struct ActionTransf; struct ActionHitted; struct ActionSpinKick; struct ActionDie; struct ActionMine;
#pragma pack(push, 1)
/*__ACTION_STRUCT__*/
#pragma pack(pop)

enum CMType : uint8_t { /*__CLIENT_ENUM__*/ };
#pragma pack(push, 1)
/*__CLIENT_STRUCTS__*/
#pragma pack(pop)

enum SMType : uint8_t { /*__SERVER_ENUM__*/ };
#pragma pack(push, 1)
/*__SERVER_STRUCTS__*/
#pragma pack(pop)

static void print_hex(const char *name, const void *p, size_t n)
{
    std::printf("%s\t", name);
    auto q = static_cast<const uint8_t *>(p);
    for(size_t i = 0; i < n; ++i){ std::printf("%02x", q[i]); }
    std::printf("\n");
}

int main()
{
    std::printf("HELPER\tStaticBuffer64\t%zu\n", sizeof(StaticBuffer<64>));
    std::printf("HELPER\tStaticBuffer128\t%zu\n", sizeof(StaticBuffer<128>));
    std::printf("HELPER\tStaticVectorU32_512\t%zu\n", sizeof(StaticVector<uint32_t, 512>));
    std::printf("HELPER\tStaticVectorU64_128\t%zu\n", sizeof(StaticVector<uint64_t, 128>));
    std::printf("ACTION_OFFSET\tx\t%zu\n", offsetof(ActionNode, x));
    std::printf("ACTION_OFFSET\ty\t%zu\n", offsetof(ActionNode, y));
    std::printf("ACTION_OFFSET\taimX\t%zu\n", offsetof(ActionNode, aimX));
    std::printf("ACTION_OFFSET\taimY\t%zu\n", offsetof(ActionNode, aimY));
    std::printf("ACTION_OFFSET\taimUID\t%zu\n", offsetof(ActionNode, aimUID));
    std::printf("ACTION_OFFSET\textParam\t%zu\n", offsetof(ActionNode, extParam));
/*__STRUCT_SIZES__*/

    ActionNode a{};
    a.type = 0x1f;
    print_hex("ACTION_BITS\ttype", &a, 4);
    std::memset(&a, 0, sizeof(a));
    a.speed = 0x1ff;
    print_hex("ACTION_BITS\tspeed", &a, 4);
    std::memset(&a, 0, sizeof(a));
    a.direction = 0x1f;
    print_hex("ACTION_BITS\tdirection", &a, 4);

    CMChatMessageHeader h{};
    h.hasRef = 1;
    print_hex("CHAT_HEADER_BITS\thasRef", &h, sizeof(h));
    std::memset(&h, 0, sizeof(h));
    h.refID = 1;
    print_hex("CHAT_HEADER_BITS\trefID", &h, sizeof(h));

    CMRequestLatestChatMessage r{};
    r.limitCount = 0x3fffffff;
    print_hex("LATEST_CHAT_BITS\tlimitCount", reinterpret_cast<uint8_t *>(&r) + sizeof(r) - 4, 4);
    std::memset(&r, 0, sizeof(r));
    r.includeSend = 1;
    print_hex("LATEST_CHAT_BITS\tincludeSend", reinterpret_cast<uint8_t *>(&r) + sizeof(r) - 4, 4);
    std::memset(&r, 0, sizeof(r));
    r.includeRecv = 1;
    print_hex("LATEST_CHAT_BITS\tincludeRecv", reinterpret_cast<uint8_t *>(&r) + sizeof(r) - 4, 4);

    SMOnlineOK o{};
    o.gender = 1;
    print_hex("ONLINE_BITS\tgender", reinterpret_cast<uint8_t *>(&o) + offsetof(SMOnlineOK, mapUID) - 1, 1);
    std::memset(&o, 0, sizeof(o));
    o.job = 7;
    print_hex("ONLINE_BITS\tjob", reinterpret_cast<uint8_t *>(&o) + offsetof(SMOnlineOK, mapUID) - 1, 1);

    SMCORecord c{};
    c.Player.gender = 1;
    print_hex("CORECORD_BITS\tgender", reinterpret_cast<uint8_t *>(&c) + 44, 1);
    std::memset(&c, 0, sizeof(c));
    c.Player.job = 7;
    print_hex("CORECORD_BITS\tjob", reinterpret_cast<uint8_t *>(&c) + 44, 1);
    std::memset(&c, 0, sizeof(c));
    c.Player.teamLeader = 1;
    print_hex("CORECORD_BITS\tteamLeader", reinterpret_cast<uint8_t *>(&c) + 44, 1);
    std::memset(&c, 0, sizeof(c));
    c.Player.stall = 1;
    print_hex("CORECORD_BITS\tstall", reinterpret_cast<uint8_t *>(&c) + 44, 1);
}
