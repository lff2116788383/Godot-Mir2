# Storage layer

This package is the B2 storage foundation for the Go server.

## Implemented

- Exact embedded legacy schema: 22 tables, 11 indexes and the guild-leader trigger.
- Pure-Go SQLite through `modernc.org/sqlite`.
- Legacy scalar functions:
  - `jobValid`
  - `SYS_PLY_VIEWR`
  - `checkAuctionCategory`
- DSN-level `foreign_keys`, `busy_timeout`, WAL and synchronous settings.
- Account and character repositories.
- Inventory, secured item, wear, belt, learned magic and player config repositories.
- Character Lua variable repository.
- Gold repository with atomic add/remove semantics.
- Quest dynamic-table/field repository.
- Chat group/message/friend/block repository.
- Guild membership, invite, rank, notice and war repository.
- Auction, stall, delivery and atomic direct-trade repositories.
- `cerealf`-compatible BLOB helpers using the protocol package.
- Content manifest SHA-256 verification.
- Schema, foreign-key, UDF, transaction, BLOB and repository tests.

## Batch boundary

B2 owns persistence and data-access foundations. The following remain with their
gameplay/data-owning batches:

- item/monster/magic/map record table parsing: B4/B5;
- auction purchase tax/delivery orchestration and stall buyer payment: B6;
- `serdesmsg.cpp` domain helper methods: their owning gameplay batches;
- real live client/server packet capture: B8.
