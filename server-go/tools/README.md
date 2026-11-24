# 工具

计划包含：

- `freeze-baseline.ps1`：冻结 C++ 参考基线；
- `generate-source-index.ps1`：生成文件级迁移索引；
- 后续记录表转换器和协议 fixture 生成器。
## Protocol fixtures

- `protocol-fixtures/generate-cpp-layout.ps1`：生成 packed message ABI 尺寸；
- `protocol-fixtures/generate-message-table.ps1`：生成消息 ID 和 framing 属性；
- `protocol-fixtures/generate-serdes-fixtures.ps1`：生成 cereal PortableBinary golden 样本。
- `protocol-fixtures/generate-framing-fixtures.ps1`：生成完整 client/server framing golden packet。
