#include <cstdint>
#include <cstdio>
#include <string>
#include <vector>

static void append_varint(std::vector<uint8_t> &out, uint64_t value)
{
    while(value >= 0x80){
        out.push_back(static_cast<uint8_t>(value) | 0x80);
        value >>= 7;
    }
    out.push_back(static_cast<uint8_t>(value));
}

static std::vector<uint8_t> frame(
        uint8_t messageID,
        int kind,
        const std::vector<uint8_t> &payload,
        uint64_t responseID)
{
    std::vector<uint8_t> packet;
    uint8_t head = messageID & 0x7f;
    if(responseID){ head |= 0x80; }
    packet.push_back(head);
    if(responseID){ append_varint(packet, responseID); }

    switch(kind){
        case 0:
            {
                break;
            }
        case 1:
            {
                const bool wide = payload.size() >= 256;
                const size_t maskLen = wide ? (payload.size() + 63) / 64 : (payload.size() + 7) / 8;
                std::vector<uint8_t> mask(maskLen, 0);
                std::vector<uint8_t> compressed;
                if(wide){
                    for(size_t i = 0; i < payload.size(); i += 8){
                        const size_t n = std::min<size_t>(8, payload.size() - i);
                        bool nonZero = false;
                        for(size_t j = 0; j < n; ++j){ nonZero = nonZero || payload[i + j]; }
                        if(nonZero){
                            mask[i / 64] |= 1 << ((i % 64) / 8);
                            compressed.insert(compressed.end(), payload.begin() + i, payload.begin() + i + n);
                        }
                    }
                }
                else{
                    for(size_t i = 0; i < payload.size(); ++i){
                        if(payload[i]){
                            mask[i / 8] |= 1 << (i % 8);
                            compressed.push_back(payload[i]);
                        }
                    }
                }
                append_varint(packet, compressed.size());
                packet.insert(packet.end(), mask.begin(), mask.end());
                packet.insert(packet.end(), compressed.begin(), compressed.end());
                break;
            }
        case 2:
            {
                packet.insert(packet.end(), payload.begin(), payload.end());
                break;
            }
        case 3:
            {
                append_varint(packet, payload.size());
                packet.insert(packet.end(), payload.begin(), payload.end());
                break;
            }
        default:
            {
                std::fprintf(stderr, "bad kind %d\n", kind);
                std::exit(2);
            }
    }
    return packet;
}

static std::vector<uint8_t> sparsePattern(size_t size, size_t period, uint8_t seed)
{
    std::vector<uint8_t> value(size, 0);
    for(size_t i = 0; i < size; ++i){
        if(i % period == 0){ value[i] = static_cast<uint8_t>(seed + i); }
    }
    return value;
}

static void print_hex(const std::vector<uint8_t> &buf)
{
    for(uint8_t value: buf){ std::printf("%02x", value); }
}

static void dump(
        const char *name,
        const char *direction,
        uint8_t messageID,
        int kind,
        const std::vector<uint8_t> &payload,
        uint64_t responseID)
{
    const auto packet = frame(messageID, kind, payload, responseID);
    std::printf("%s\t%s\t%u\t%d\t%zu\t%llu\t", name, direction, messageID, kind, payload.size(), static_cast<unsigned long long>(responseID));
    print_hex(payload);
    std::printf("\t");
    print_hex(packet);
    std::printf("\n");
}

int main()
{
    dump("server_ping_raw", "server", 3, 2, {0x78, 0x56, 0x34, 0x12}, 0);
    dump("server_error_empty", "server", 2, 0, {}, 0);
    dump("server_login_error_xor", "server", 6, 1, {0x01}, 0);
    dump("server_online_xor", "server", 20, 1, sparsePattern(177, 29, 1), 0);
    dump("client_login_xor", "client", 2, 1, sparsePattern(136, 17, 3), 0);
    dump("client_create_chat_group_xor64", "client", 31, 1, sparsePattern(2184, 257, 7), 0);
    dump("client_ping_response", "client", 1, 2, {0x78, 0x56, 0x34, 0x12}, 300);
    dump("client_chat_variable", "client", 43, 3, {
        0x88,0x77,0x66,0x55,0x44,0x33,0x22,0x11,
        0x03,0x00,0x00,0x00,0x00,0x00,0x00,0x00,
        'h','e','l','l','o'
    }, 0);
}
