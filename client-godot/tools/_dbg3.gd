extends Node
func run(main: Node) -> void:
	var p = main.get("_player")
	print("start grid=", p.grid, " moving=", p.is_moving())
	var target := Vector2i(p.grid.x - 6, p.grid.y - 6)
	print("target=", target)
	p.grid_checker = main.call("_check_path_grid")
	var f := PathFinder.new(main.call("_check_path_grid"), 1, 1)
	var path := f.find_path(p.grid, target, p.facing)
	print("path len=", path.size(), " path=", path)
	if path.size() > 0:
		print("first=", path[0], " last=", path[-1])
		var d := PathFinder.get_off_dir(p.grid.x, p.grid.y, path[0].x, path[0].y)
		print("dir to first=", d)
	get_tree().quit(0)
