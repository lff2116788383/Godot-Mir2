package game

import (
	"context"
	"log/slog"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// ==================== 仓库（寄存 / 取回）====================
//
// 【依据】player.cpp:2572 secureItem / :2583 removeSecuredItem
//
//	寄存：dbSecureItem(itemID, seqID)  →  removeInventoryItem
//	取回：dbRetrieveSecuredItem        →  addInventoryItem  →  SM_REMOVESECUREDITEM
//
// ★ 两边的「顺序」是相反的，这是最容易写错的地方：
//
//	寄存时**先写库再删背包**：
//	  先删背包再写库，写库失败物品就凭空消失了。
//	取回时**先写库（原子地删除）再进背包**：
//	  先删库再进背包，进背包失败物品也消失了。
//
//	原版的 dbRetrieveSecuredItem 内部是一条 delete + 返回被删的行，
//	天然原子。Go 侧 storage.TakeSecuredItem 也是同样语义。

// SecuredHandler 处理仓库寄存与取回。
type SecuredHandler struct {
	Players   *PlayerManager
	Broadcast Broadcaster
	Logger    *slog.Logger
	// Items 物品表（算格子数上限、校验物品存在）
	Items *ItemDB
	Repo  SecuredRepository
}

// SecuredRepository 是仓库持久化的最小接口。
type SecuredRepository interface {
	PutSecuredItem(ctx context.Context, dbid uint32, item protocol.SDItem) error
	TakeSecuredItem(ctx context.Context, dbid, itemID, seqID uint32) (protocol.SDItem, bool, error)
	LoadSecuredItems(ctx context.Context, dbid uint32) ([]protocol.SDItem, error)
	RemoveInventoryItem(ctx context.Context, dbid, itemID, seqID uint32) (bool, error)
}

// Secure 把背包里的一件物品寄存到仓库（NPC 脚本调用）。
//
// 返回 true 表示成功。
func (h *SecuredHandler) Secure(ctx context.Context, p *Player, itemID, seqID uint32) bool {
	if p == nil || h.Repo == nil {
		return false
	}
	idx, it := p.findItem(itemID, seqID)
	if it == nil {
		return false
	}

	// 1. 先写库（含持久值，否则取回后装备恢复满持久）
	if err := h.Repo.PutSecuredItem(ctx, p.DBID, toSDItem(*it)); err != nil {
		h.log("secure item failed", err)
		return false
	}

	// 2. 再删背包
	p.Inventory = append(p.Inventory[:idx], p.Inventory[idx+1:]...)

	// 3. 删库里的背包记录
	if _, err := h.Repo.RemoveInventoryItem(ctx, p.DBID, itemID, seqID); err != nil {
		h.log("remove inventory after secure failed", err)
	}
	return true
}

// Retrieve 从仓库取回一件物品（CM_REQUESTRETRIEVESECUREDITEM）。
//
// 原版 net_CM_REQUESTRETRIEVESECUREDITEM 只有两步：
// 摆摊检查 + removeSecuredItem。**不做距离校验** ——
// 因为这条消息只能从 NPC 对话框里发（服务端信任 NPC 已做过距离检查）。
func (h *SecuredHandler) Retrieve(ctx context.Context, channelID uint64, itemID, seqID uint32) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	if h.Repo == nil {
		return nil
	}

	// 从仓库原子取出（库里删掉的同时返回内容）
	item, found, err := h.Repo.TakeSecuredItem(ctx, p.DBID, itemID, seqID)
	if err != nil {
		h.log("take secured item failed", err)
		return nil
	}
	if !found {
		return nil
	}

	// 进背包（带持久值）
	p.Inventory = append(p.Inventory, fromSDItem(item))

	// 推背包 + 仓库列表各一份：
	//
	//	SM_INVENTORY          背包变了
	//	SM_REMOVESECUREDITEM  仓库列表里要删掉这一项
	//
	// 只推前者的话，NPC 仓库面板里那一行还在，玩家会重复点「取回」。
	h.notifyInventory(ctx, p)
	h.notifyRemoved(ctx, p, itemID, seqID)
	return nil
}

// ShowList 把仓库内容推给客户端（NPC 点「寄存」时由脚本调 uidPostSecuredItemList）。
func (h *SecuredHandler) ShowList(ctx context.Context, p *Player) error {
	if p == nil || h.Repo == nil {
		return nil
	}
	items, err := h.Repo.LoadSecuredItems(ctx, p.DBID)
	if err != nil {
		h.log("load secured items failed", err)
		return nil
	}
	// SM_SHOWSECUREDITEMLIST 是变长消息，body 是 cerealf 序列化的 SDShowSecuredItemList
	payload, err := protocol.CerealSerialize(protocol.SDShowSecuredItemList{ItemList: items})
	if err != nil {
		return nil
	}
	if h.Broadcast != nil && p.ChannelID != 0 {
		_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_SHOWSECUREDITEMLIST, payload, 0)
	}
	return nil
}

func (h *SecuredHandler) notifyInventory(ctx context.Context, p *Player) {
	if h.Broadcast == nil || p.ChannelID == 0 {
		return
	}
	list := make([]protocol.SDItem, 0, len(p.Inventory))
	for _, it := range p.Inventory {
		list = append(list, toSDItem(it))
	}
	payload, err := protocol.CerealSerialize(protocol.SDInventory{ItemList: list})
	if err != nil {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_INVENTORY, payload, 0)
}

func (h *SecuredHandler) notifyRemoved(ctx context.Context, p *Player, itemID, seqID uint32) {
	if h.Broadcast == nil || p.ChannelID == 0 {
		return
	}
	payload, err := protocol.AppendWirePayload(nil, protocol.SMRemoveSecuredItem{
		ItemID: itemID, SeqID: seqID,
	})
	if err != nil {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_REMOVESECUREDITEM, payload, 0)
}

func (h *SecuredHandler) log(msg string, err error) {
	if h.Logger == nil {
		return
	}
	if err != nil {
		h.Logger.Warn(msg, "error", err)
		return
	}
	h.Logger.Debug(msg)
}
