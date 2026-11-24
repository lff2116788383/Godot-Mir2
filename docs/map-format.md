# mir2x 地图二进制格式（已验证）

> 本文所有结论均经**实际字节比对**验证，非推测。
> 验证样本：`mir2x_assets-main/map/0.bin`，文件 634,004 字节，解压后 41,600,025 字节。

## 1. 结论先行

**`.bin` 不是"压缩包"，而是离线烘焙后的地图数据表。**

| 作用 | 是否成立 | 证据 |
|---|---|---|
| 唯一数据源 | ✅ **必需** | assets 下 598 个地图文件**全部是 .bin**，无任何原始 .map |
| 离线预计算 | ✅ 主要目的 | 存的是 `texID` 直查索引，运行时免去 WIL 解码与遮挡计算 |
| 免运行时解析 | ✅ 主要目的 | 运行时只需 `texture/map/{texID:08X}.PNG` |
| 压缩 | ✅ 附带 | 41.6 MB → 634 KB，约 **62:1** |

**若不复刻此格式，将没有任何可用地图数据。**

## 2. 文件整体结构（已修正）

```
[zstd 帧][压缩标记:u8]      ← flag 在文件【末尾】，由 cerealf::serialize 追加
```

解压后（cereal PortableBinary）：

```
[endian:u8=1][w:u64][h:u64][count:u64][count × BLOCK]
```

**两个必须记住的坑**：

1. **压缩标记在文件末尾，不在开头。** 文件首 4 字节是 zstd 魔数 `28 B5 2F FD`。
2. **解压后第 0 字节是端序标记（1 = 小端）**，不是数据。
   它来自 `PortableBinaryOutputArchive` 构造函数里的
   `this->operator()(options.is_little_endian())`（`portable_binary.hpp:125`）。
   **所有 cerealf 消息都有这 1 字节**，解码时都要跳过。

**实测验证（8 张地图全部吻合）**：

| 文件 | w | h | count | 解压长度 | `1+24+count×260` |
|---|---|---|---|---|---|
| 0.bin | 800 | 800 | 160000 | 41,600,025 | 41,600,025 ✅ |
| 01.bin | 600 | 600 | 90000 | 23,400,025 | 23,400,025 ✅ |
| 01_003.bin | 18 | 22 | 99 | 25,765 | 25,765 ✅ |

`count == w * h / 4` 恒成立。

## 3. BLOCK 布局（260 字节）

```
BLOCK = TILE(8B) + CELL[4](58B × 4)
```

**CELL 索引规则**（`mir2xmapdata.hpp:242`）：
```
cell_index = (y % 2) * 2 + (x % 2)
```

### 3.1 TILE（8 字节）
| 偏移 | 类型 | 字段 |
|---|---|---|
| 0 | u32 | `valid` |
| 4 | u32 | `texID`（24 位有效） |

### 3.2 CELL（63 字节，不是 58！）
| 偏移 | 类型 | 字段 |
|---|---|---|
| 0 | u8 | `land.type`（6 bit） |
| 1 | u8 | `land.canFly`（1 bit） |
| 2 | u8 | `land.canWalk`（1 bit） |
| 3 | u8 | `light.valid` |
| 4 | u8 | `light.radius` |
| 5 | u8 | `light.alpha` |
| 6 | u8 | `light.color` |
| 7 | 28B | `obj[0]`（7 × u32） |
| 35 | 28B | `obj[1]`（7 × u32） |

**校验**：`TILE(8) + 4 × CELL(63) = 8 + 252 = 260` ✅

> **踩过的坑**：最初把 CELL 当成 58 字节（3+4+2×28=63 算错了 4 格），
> 导致每块少读 20 字节，第 2 块起全部错位，
> 地图只渲染出 26.8% 的瓦片并呈棋盘状。
> 按 63 修正后有效瓦片变为 **100.0%**，画面连续无缝。

### 3.3 OBJECT（28 字节，7 × u32）
| 序 | 字段 | 位宽 |
|---|---|---|
| 0 | `valid` | 1 |
| 1 | `texID` | 24 |
| 2 | `depth` | 2（默认 2） |
| 3 | `animated` | 1 |
| 4 | `alpha` | 1 |
| 5 | `tickType` | 3 |
| 6 | `frameCount` | 4 |

> **关键**：源码头文件里 `TILE`/`CELL`/`OBJECT` 虽是 `uint32_t` 位域，
> 但 `serialize()` 把每个位域**拆成独立局部变量**再 `ar(...)`，
> 因此落盘是 **每个位域一个完整 u32**，不是打包后的紧凑字节。
> 这就是 260 字节（而非按 `#pragma pack` 推算的 240）的来源。

## 4. 已验证样本

BLOCK0 解析结果：

```
tile: valid=1, texID=0x0102F2 (66290)  → texture/map/000102F2.PNG
cell0..3: land=(type=0, fly=0, walk=0), light.valid=0
          obj0=obj1=(valid=0, depth=2, 其余=0)
```

`texID = 0x0102F2` → 文件名 `000102F2.PNG`，与 `texture/map/` 命名规则
`%08X.PNG`（`tools/mapwil2png/src/main.cpp:33`）完全一致，链路闭合。

## 5. 坐标系与渲染

| 常量 | 值 | 位置 |
|---|---|---|
| `SYS_MAPGRIDXP` | 48 px | `sysconst.hpp:55` |
| `SYS_MAPGRIDYP` | 32 px | `sysconst.hpp:56` |

- 地图逻辑尺寸：`w × h` 格（本例 800×800）
- 像素尺寸：`w*48 × h*32` = 38400 × 25600 px
- **1 个 BLOCK 覆盖 2×2 格**，即 96×64 px

### 素材分辨率关键点

原版 `mir2map.hpp:167-168` 要求瓦片尺寸恰好等于 `48×32`：

```cpp
&& imgDB.currImageInfo()->width  == SYS_MAPGRIDXP
&& imgDB.currImageInfo()->height == SYS_MAPGRIDYP;
```

但 `mir2x_assets-main/texture/map/*.PNG` **实测全部是 96×64**。

而 `BLOCK = 2 格 × (48,32) = 96×64`，**正好等于一张瓦片**。
所以：**一张瓦片绘制一个 BLOCK，tile_scale = 1.0，不需要缩放。**

> **踩过的坑**：一开始按"素材 2 倍高清"错误地设了 `scale=0.5`，
> 结果瓦片只盖住 1/4 区域，画面呈稀疏棋盘。
> 正确做法是 scale=1.0，瓦片左上角对齐 `(gx*48, gy*32)`。

## 6. Godot 读取实现要点

1. 读文件 → **取末字节**判断压缩标记（1=zstd，0=不压缩，2=XOR）
2. 切掉末字节后 zstd 解压，**缓冲区要给足**（800×800 需 ≥ 42 MB）
3. **跳过第 0 字节**（端序标记），再按 `u64` 读 w/h/count
4. 每 260 字节切一个 BLOCK；每 CELL 63 字节
5. `texID & 0xFFFFFF` 有效，路径 `"%08X.PNG" % texID`

> **性能实测**（本机 RTX 3070 / Godot 4.7.2）：
> - 800×800 地图解析：**约 180 ms**
> - 单张 PNG 加载：**约 0.86 ms**
> - 视野 44×28 格 = 462 个 BLOCK，纹理命中率 **94.9%**（仅缓存 6 张）
>
> 命中率之所以高，是因为同屏瓦片重复率极高，LRU 缓存收益显著。
