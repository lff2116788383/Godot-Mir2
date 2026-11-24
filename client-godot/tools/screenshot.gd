extends SceneTree
## 自动截图工具（不依赖 Autoload，直接装场景树）
## 用法：godot --path . --script tools/screenshot.gd -- <输出> <等待帧数>

const ASSETS_DEFAULT := "E:/Desktop/game/mir2x_assets-main"


func _init() -> void:
	var args := OS.get_cmdline_user_args()
	var out_path := "res://shot.png"
	var wait_frames := 40
	if args.size() >= 1:
		out_path = args[0]
	if args.size() >= 2:
		wait_frames = int(args[1])

	var assets := OS.get_environment("MIR2X_ASSETS")
	if assets.is_empty():
		assets = ASSETS_DEFAULT

	# --script 模式下 Autoload 不生效，手动挂载
	var game_script: GDScript = load("res://scripts/core/game.gd")
	var tex_script: GDScript = load("res://scripts/data/tex_cache.gd")
	var game_node: Node = game_script.new()
	var tex_node: Node = tex_script.new()
	game_node.name = "Game"
	tex_node.name = "TexCache"
	root.add_child(game_node)
	root.add_child(tex_node)
	game_node.set("assets_root", assets)
	print("[shot] assets=", assets)

	var scene: PackedScene = load("res://scenes/main.tscn")
	var main: Node = scene.instantiate()
	root.add_child(main)

	for i in range(wait_frames):
		await process_frame

	await RenderingServer.frame_post_draw
	var img := root.get_texture().get_image()
	var err := img.save_png(out_path)
	print("[shot] save ", out_path, " err=", err, " ", img.get_width(), "x", img.get_height())
	quit(0)
