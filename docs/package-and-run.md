# Godot-Mir2 导出与运行

## Windows 导出

```powershell
powershell -ExecutionPolicy Bypass -File tools/export_client.ps1 `
  -GodotBin "E:\Program Files\Godot\Godot_v4.7.2-stable_win64.exe"
```

默认输出：

```text
build/Godot-Mir2.exe
build/run-client.ps1
```

`export_presets.cfg` 使用 Windows Desktop 预设，并启用 `embed_pck`。

## 资源目录

当前客户端仍然外置加载 mir2x assets。运行前必须设置：

```powershell
$env:MIR2X_ASSETS = "E:\Desktop\game\mir2x_assets-main"
```

或者直接运行生成的 `build/run-client.ps1`。

## 尚未自动化

- 将 1.34GB 资源目录打进安装包
- 安装包签名、增量更新和启动器
- Linux/macOS 导出预设
