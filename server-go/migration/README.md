# 文件级迁移索引

`source-index.csv` 由 B0 基线脚本生成，记录 C++ 源文件的哈希、目标包、批次和状态。

脚本不会自动覆盖已经人工填写的 `go_package`、`batch`、`status` 和 `notes`。更新索引时应先重新生成到临时文件再人工合并。
