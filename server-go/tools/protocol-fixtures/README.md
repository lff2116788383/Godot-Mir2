# Old protocol fixture tools

These tools extract the packed C++ message declarations and produce verified layout/metadata used by the Go protocol port.

## Layout probe

```powershell
pwsh ./tools/protocol-fixtures/generate-cpp-layout.ps1
```

It:

- reads the authoritative C++ headers;
- extracts enums, packed message structs and `ActionNode`;
- builds a small standalone C++ layout probe so no full mir2x C++ build is required;
- writes `internal/protocol/testdata/cpp-layout.json`.

The probe intentionally mirrors only the primitive helpers (`StaticBuffer`, `StaticVector`) and constants needed for layout. Message declarations themselves are extracted from the source files.

## Message metadata

```powershell
pwsh ./tools/protocol-fixtures/generate-message-table.ps1
```

It reads the `_RSVD_register_clientmsg` and `_RSVD_register_servermsg` tables plus the C++ layout JSON and generates:

- `internal/protocol/messages_generated.go`
- all legacy message IDs;
- message kind and fixed payload size metadata.

## Cereal object fixtures

```powershell
pwsh ./tools/protocol-fixtures/generate-serdes-fixtures.ps1
```

It compiles `serdes_fixture_probe.cpp` with cereal and writes raw `PortableBinaryOutputArchive` samples to:

- `internal/protocol/testdata/serdes-golden.json`

The probe mirrors the first batch of `serdesmsg.hpp` objects. Extend the probe whenever another object is migrated.

All generated outputs are checked in so normal Go development does not require the C++ toolchain.
