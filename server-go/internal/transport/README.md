# Legacy TCP transport

This package owns client connections using the B1 wire protocol:

- TCP accept loop and session IDs;
- packet framing through `protocol.PacketDecoder`;
- per-session writer queue;
- response-ID preservation;
- read/write deadlines and connection limits;
- malformed-packet isolation;
- graceful close and lifecycle callbacks;
- message dispatcher.

B3 ships a bootstrap dispatcher with `CM_PING` -> `SM_PING`. Full login/game
handlers remain owned by B5 and are registered through the dispatcher.
