package game

import (
	"sync/atomic"

	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
)

// ==================== 拍卖分类匹配器 ====================
//
// 【依据】server/src/server.cpp:319 checkAuctionCategory
//
// 拍卖列表按「大类」筛选（衣服/武器/项链/头盔/戒指/手镯/鞋/药水/书/其他）。
// 这个筛选在 SQL 里做，靠 SQLite 自定义函数 checkAuctionCategory(itemID, category)。
//
// ★★ 这里踩过一个「静默空列表」的坑，值得完整留档。★★
//
// storage 的 checkAuctionCategory 需要一个「itemID → 是否属于某类」的函数，
// 而物品表是 game 包载的（storage 不能 import game）。开库时 main
// 没有传这个匹配器，于是那个 SQL 函数**恒返回 0**：
//
//	where a.fld_expiretime > ? and checkAuctionCategory(a.fld_itemid, ?) = 1
//	                                                                  ↑ 永远不成立
//
// 后果是**拍卖行列表永远为空**，而上架/购买都正常（它们不走这个条件）。
// 表现极具迷惑性：玩家看到的是「拍卖行没有人挂东西」，
// 而不是任何错误提示 —— 排查时很容易以为是「没数据」。
//
// 修法：本文件在物品表加载后把匹配器装进 storage，见 InstallAuctionCategories。

// 拍卖分类常量（protocoldef.hpp:202-217 AUCTIONCAT_*）。
const (
	AuctionCatAll      = 1
	AuctionCatDress    = 2
	AuctionCatWeapon   = 3
	AuctionCatNecklace = 4
	AuctionCatHelmet   = 5
	AuctionCatRing     = 6
	AuctionCatArmRing  = 7
	AuctionCatShoes    = 8
	AuctionCatPotion   = 9
	AuctionCatBook     = 10
	AuctionCatOther    = 11
	AuctionCatEnd      = 12
)

// 防重复安装（多次调用只装一次）。
var auctionCategoriesInstalled atomic.Bool

// InstallAuctionCategories 把物品表接入 storage 的拍卖分类函数。
//
// 必须在 LoadItemDB 之后、拍卖功能可用之前调用一次。
//
// ★ 逐条照 server.cpp:340-358 的 categoryMatch：
//
//	DRESS    isDress()
//	WEAPON   isWeapon()
//	NECKLACE wearable(WLG_NECKLACE)
//	HELMET   isHelmet()
//	RING     isRing()
//	ARMRING  wearable(WLG_ARMRING0)
//	SHOES    wearable(WLG_SHOES)
//	POTION   isPotion() || isDope() || isPowder()
//	BOOK     isBook()
//
// ★ 「其他」不是「没有类别」，而是「不属于以上任何一类」——
//
//	原版是从 ALL+1 遍历到 OTHER-1，只要有任意一类匹配就返回 0。
//	写成「兜底 true」会让其他类的物品重复出现在每一类里。
func InstallAuctionCategories(items *ItemDB) {
	if items == nil || auctionCategoriesInstalled.Load() {
		return
	}
	auctionCategoriesInstalled.Store(true)

	storage.SetItemCategoryMatch(func(itemID, category int64) bool {
		if itemID <= 0 || category <= 0 {
			return false
		}
		rec, ok := items.Get(uint32(itemID))
		if !ok {
			return false
		}

		matches := func(cat int64) bool {
			switch cat {
			case AuctionCatDress:
				return rec.Type == TypeDress
			case AuctionCatWeapon:
				return rec.Type == TypeWeapon
			case AuctionCatNecklace:
				return rec.Wearable(WLGNecklace)
			case AuctionCatHelmet:
				return rec.Type == TypeHelmet
			case AuctionCatRing:
				return rec.Type == TypeRing
			case AuctionCatArmRing:
				return rec.Wearable(WLGArmRing0)
			case AuctionCatShoes:
				return rec.Wearable(WLGShoes)
			case AuctionCatPotion:
				return rec.Type == TypePotion || rec.Type == TypeDope || rec.Type == TypePowder
			case AuctionCatBook:
				return rec.IsBook()
			default:
				return false
			}
		}

		switch category {
		case AuctionCatAll:
			return true
		case AuctionCatOther:
			for cat := int64(AuctionCatAll + 1); cat < AuctionCatOther; cat++ {
				if matches(cat) {
					return false
				}
			}
			return true
		default:
			return matches(category)
		}
	})
}
