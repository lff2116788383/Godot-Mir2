# 参考基线

本目录固定 Go 复刻所依据的 C++ 服务端输入。

执行以下命令可重新生成：

```powershell
pwsh ./scripts/freeze-baseline.ps1
```

主要文件：

- `source.path.txt`：参考仓库绝对路径；
- `source.commit.txt`：Git 提交与状态；
- `source.status.txt`：冻结时工作树状态；
- `source.patch`：已跟踪文件的工作树修改；
- `source.untracked.txt`：未跟踪文件清单；
- `source-files.sha256`：服务端、common 和构建输入的 SHA-256 清单；
- `asset-status.txt`：资源仓库探测结果。

注意：当前源工作树不是干净状态，因此提交号和 `source.patch` 必须一起保存。
