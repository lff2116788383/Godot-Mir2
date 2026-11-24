// Command seed 建一个可联调的测试账号与角色。
//
// 用法：go run ./tools/seed -db mir2x.db3 -account test@test.com -password 123456
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
)

func main() {
	dbPath := flag.String("db", "mir2x.db3", "database path")
	account := flag.String("account", "test@test.com", "account name")
	password := flag.String("password", "123456", "account password")
	charName := flag.String("char", "测试角色", "character name")
	job := flag.Int("job", 1, "job id (1=warrior 2=wizard 3=taoist)")
	gender := flag.Int("gender", 1, "gender (1=male 0=female)")
	mapID := flag.Int("map", 1, "spawn map id")
	mapX := flag.Int("x", 400, "spawn x")
	mapY := flag.Int("y", 402, "spawn y")
	flag.Parse()

	ctx := context.Background()
	store, err := storage.Open(ctx, storage.OpenOptions{Path: *dbPath, CreateIfMissing: true})
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	accounts := storage.NewAccountRepository(store)
	if err := accounts.Create(ctx, *account, *password); err != nil {
		fmt.Println("account:", err)
	} else {
		fmt.Printf("account %q created\n", *account)
	}

	chars := storage.NewCharacterRepository(store)
	if _, err := chars.Create(ctx, storage.CreateCharacterParams{
		Account: *account,
		Name:    *charName,
		Gender:  uint8(*gender),
		Job:     uint8(*job),
		Map:     uint32(*mapID),
		MapX:    int32(*mapX),
		MapY:    int32(*mapY),
	}); err != nil {
		fmt.Println("character:", err)
	} else {
		fmt.Printf("character %q created at map %d (%d,%d)\n", *charName, *mapID, *mapX, *mapY)
	}
}
