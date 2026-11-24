package protocol

import "sort"

// SDItem is the serialized inventory item representation.
type SDItem struct {
	ItemID      uint32
	SeqID       uint32
	Count       uint64
	Duration    [2]uint64
	ExtAttrList map[int32]string
}

func (v SDItem) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.ItemID)
	writer.Uint32(v.SeqID)
	writer.Uint64(v.Count)
	writer.Uint64(v.Duration[0])
	writer.Uint64(v.Duration[1])

	keys := make([]int32, 0, len(v.ExtAttrList))
	for key := range v.ExtAttrList {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	writer.SizeTag(uint64(len(keys)))
	for _, key := range keys {
		writer.Int32(key)
		writer.String(v.ExtAttrList[key])
	}
	return writer.Err()
}

func (v *SDItem) UnmarshalPortable(reader *PortableReader) error {
	v.ItemID = reader.Uint32()
	v.SeqID = reader.Uint32()
	v.Count = reader.Uint64()
	v.Duration[0] = reader.Uint64()
	v.Duration[1] = reader.Uint64()
	count := readPortableCount(reader, 1<<20, "item extra attribute")
	if reader.Err() != nil {
		return reader.Err()
	}
	if count == 0 {
		v.ExtAttrList = nil
	} else {
		v.ExtAttrList = make(map[int32]string, count)
	}
	for i := 0; i < count; i++ {
		key := reader.Int32()
		v.ExtAttrList[key] = reader.String()
	}
	return reader.Err()
}

func (v SDItem) ItemIDSeq() uint64 {
	return uint64(v.ItemID)<<32 | uint64(v.SeqID)
}

type SDDropItem struct {
	X    int32
	Y    int32
	Item SDItem
}

func (v SDDropItem) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.X)
	writer.Int32(v.Y)
	return v.Item.MarshalPortable(writer)
}

func (v *SDDropItem) UnmarshalPortable(reader *PortableReader) error {
	v.X = reader.Int32()
	v.Y = reader.Int32()
	return v.Item.UnmarshalPortable(reader)
}

type SDDropItemList struct {
	X        int32
	Y        int32
	ItemList []SDItem
}

func (v SDDropItemList) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.X)
	writer.Int32(v.Y)
	writer.SizeTag(uint64(len(v.ItemList)))
	for _, item := range v.ItemList {
		if err := item.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDDropItemList) UnmarshalPortable(reader *PortableReader) error {
	v.X = reader.Int32()
	v.Y = reader.Int32()
	count := readPortableCount(reader, 1<<20, "drop item")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.ItemList = make([]SDItem, count)
	for i := range v.ItemList {
		if err := v.ItemList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDDropItemOdds struct {
	Item SDItem
	Odds uint64
}

func (v SDDropItemOdds) MarshalPortable(writer *PortableWriter) error {
	if err := v.Item.MarshalPortable(writer); err != nil {
		return err
	}
	writer.Uint64(v.Odds)
	return writer.Err()
}

func (v *SDDropItemOdds) UnmarshalPortable(reader *PortableReader) error {
	if err := v.Item.UnmarshalPortable(reader); err != nil {
		return err
	}
	v.Odds = reader.Uint64()
	return reader.Err()
}

type SDDropOnDie struct {
	PlayerUID        uint64
	AllowDefaultDrop bool
	ItemList         []SDDropItemOdds
}

func (v SDDropOnDie) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.PlayerUID)
	writer.Bool(v.AllowDefaultDrop)
	writer.SizeTag(uint64(len(v.ItemList)))
	for _, item := range v.ItemList {
		if err := item.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDDropOnDie) UnmarshalPortable(reader *PortableReader) error {
	v.PlayerUID = reader.Uint64()
	v.AllowDefaultDrop = reader.Bool()
	count := readPortableCount(reader, 1<<20, "drop on die")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.ItemList = make([]SDDropItemOdds, count)
	for i := range v.ItemList {
		if err := v.ItemList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDDropOnHit struct {
	ItemList []SDDropItemOdds
}

func (v SDDropOnHit) MarshalPortable(writer *PortableWriter) error {
	writer.SizeTag(uint64(len(v.ItemList)))
	for _, item := range v.ItemList {
		if err := item.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDDropOnHit) UnmarshalPortable(reader *PortableReader) error {
	count := readPortableCount(reader, 1<<20, "drop on hit")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.ItemList = make([]SDDropItemOdds, count)
	for i := range v.ItemList {
		if err := v.ItemList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDBelt struct {
	List [6]SDItem
}

func (v SDBelt) MarshalPortable(writer *PortableWriter) error {
	for _, item := range v.List {
		if err := item.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDBelt) UnmarshalPortable(reader *PortableReader) error {
	for i := range v.List {
		if err := v.List[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

func (v *SDBelt) Clear() { v.List = [6]SDItem{} }

type SDWear struct {
	List map[int32]SDItem
}

func (v SDWear) MarshalPortable(writer *PortableWriter) error {
	keys := make([]int32, 0, len(v.List))
	for key := range v.List {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	writer.SizeTag(uint64(len(keys)))
	for _, key := range keys {
		writer.Int32(key)
		if err := v.List[key].MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDWear) UnmarshalPortable(reader *PortableReader) error {
	count := readPortableCount(reader, 1<<20, "wear item")
	if reader.Err() != nil {
		return reader.Err()
	}
	if count == 0 {
		v.List = nil
	} else {
		v.List = make(map[int32]SDItem, count)
	}
	for i := 0; i < count; i++ {
		key := reader.Int32()
		var item SDItem
		if err := item.UnmarshalPortable(reader); err != nil {
			return err
		}
		v.List[key] = item
	}
	return reader.Err()
}

func (v *SDWear) SetWLItem(slot int32, item SDItem) {
	if v.List == nil {
		v.List = make(map[int32]SDItem)
	}
	v.List[slot] = item
}

func (v SDWear) GetWLItem(slot int32) (SDItem, bool) {
	item, ok := v.List[slot]
	return item, ok
}

func (v *SDWear) Clear() { v.List = nil }

type SDShowSecuredItemList struct {
	ItemList []SDItem
}

func (v SDShowSecuredItemList) MarshalPortable(writer *PortableWriter) error {
	writer.SizeTag(uint64(len(v.ItemList)))
	for _, item := range v.ItemList {
		if err := item.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDShowSecuredItemList) UnmarshalPortable(reader *PortableReader) error {
	count := readPortableCount(reader, 1<<20, "secured item")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.ItemList = make([]SDItem, count)
	for i := range v.ItemList {
		if err := v.ItemList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDSellItem struct {
	Item     SDItem
	CostList []SDCostItem
}

func (v SDSellItem) MarshalPortable(writer *PortableWriter) error {
	if err := v.Item.MarshalPortable(writer); err != nil {
		return err
	}
	writer.SizeTag(uint64(len(v.CostList)))
	for _, cost := range v.CostList {
		if err := cost.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDSellItem) UnmarshalPortable(reader *PortableReader) error {
	if err := v.Item.UnmarshalPortable(reader); err != nil {
		return err
	}
	count := readPortableCount(reader, 1<<20, "sell item cost")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.CostList = make([]SDCostItem, count)
	for i := range v.CostList {
		if err := v.CostList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDSellItemList struct {
	NPCUID uint64
	List   []SDSellItem
}

func (v SDSellItemList) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.NPCUID)
	writer.SizeTag(uint64(len(v.List)))
	for _, item := range v.List {
		if err := item.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDSellItemList) UnmarshalPortable(reader *PortableReader) error {
	v.NPCUID = reader.Uint64()
	count := readPortableCount(reader, 1<<20, "sell item")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.List = make([]SDSellItem, count)
	for i := range v.List {
		if err := v.List[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

func (v *SDSellItemList) Clear() { v.NPCUID = 0; v.List = nil }

type SDAuctionItem struct {
	AuctionID uint64
	Seller    SDChatPeer
	Note      string
	TimeLeft  uint64
	Price     uint64
	Item      SDItem
}

func (v SDAuctionItem) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.AuctionID)
	if err := v.Seller.MarshalPortable(writer); err != nil {
		return err
	}
	writer.String(v.Note)
	writer.Uint64(v.TimeLeft)
	writer.Uint64(v.Price)
	return v.Item.MarshalPortable(writer)
}

func (v *SDAuctionItem) UnmarshalPortable(reader *PortableReader) error {
	v.AuctionID = reader.Uint64()
	if err := v.Seller.UnmarshalPortable(reader); err != nil {
		return err
	}
	v.Note = reader.String()
	v.TimeLeft = reader.Uint64()
	v.Price = reader.Uint64()
	return v.Item.UnmarshalPortable(reader)
}

type SDAuctionItemList struct {
	Category int32
	ItemList []SDAuctionItem
}

func (v SDAuctionItemList) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.Category)
	writer.SizeTag(uint64(len(v.ItemList)))
	for _, item := range v.ItemList {
		if err := item.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDAuctionItemList) UnmarshalPortable(reader *PortableReader) error {
	v.Category = reader.Int32()
	count := readPortableCount(reader, 1<<20, "auction item")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.ItemList = make([]SDAuctionItem, count)
	for i := range v.ItemList {
		if err := v.ItemList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDUpdateItem struct {
	Item SDItem
}

func (v SDUpdateItem) MarshalPortable(writer *PortableWriter) error {
	return v.Item.MarshalPortable(writer)
}

func (v *SDUpdateItem) UnmarshalPortable(reader *PortableReader) error {
	return v.Item.UnmarshalPortable(reader)
}

type SDInventory struct {
	ItemList []SDItem
}

func (v SDInventory) MarshalPortable(writer *PortableWriter) error {
	writer.SizeTag(uint64(len(v.ItemList)))
	for _, item := range v.ItemList {
		if err := item.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDInventory) UnmarshalPortable(reader *PortableReader) error {
	count := readPortableCount(reader, 1<<20, "inventory item")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.ItemList = make([]SDItem, count)
	for i := range v.ItemList {
		if err := v.ItemList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

func (v SDInventory) GetItemList() []SDItem { return v.ItemList }
func (v SDInventory) Find(itemID, seqID uint32) (SDItem, bool) {
	for _, item := range v.ItemList {
		if item.ItemID == itemID && item.SeqID == seqID {
			return item, true
		}
	}
	return SDItem{}, false
}
func (v *SDInventory) Clear() { v.ItemList = nil }

type SDGrantItemList struct {
	PlayerUID uint64
	ItemList  []SDItem
}

func (v SDGrantItemList) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.PlayerUID)
	writer.SizeTag(uint64(len(v.ItemList)))
	for _, item := range v.ItemList {
		if err := item.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDGrantItemList) UnmarshalPortable(reader *PortableReader) error {
	v.PlayerUID = reader.Uint64()
	count := readPortableCount(reader, 1<<20, "grant item")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.ItemList = make([]SDItem, count)
	for i := range v.ItemList {
		if err := v.ItemList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDItemGrant struct {
	Gold      uint64
	Inventory SDInventory
}

func (v SDItemGrant) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.Gold)
	return v.Inventory.MarshalPortable(writer)
}

func (v *SDItemGrant) UnmarshalPortable(reader *PortableReader) error {
	v.Gold = reader.Uint64()
	return v.Inventory.UnmarshalPortable(reader)
}

type SDDirectTradeItem struct {
	ItemID uint32
	SeqID  uint32
	Count  uint64
}

func (v SDDirectTradeItem) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.ItemID)
	writer.Uint32(v.SeqID)
	writer.Uint64(v.Count)
	return writer.Err()
}

func (v *SDDirectTradeItem) UnmarshalPortable(reader *PortableReader) error {
	v.ItemID = reader.Uint32()
	v.SeqID = reader.Uint32()
	v.Count = reader.Uint64()
	return reader.Err()
}

func (v SDDirectTradeItem) ItemIDSeq() uint64 {
	return SDItem{ItemID: v.ItemID, SeqID: v.SeqID}.ItemIDSeq()
}

type SDDirectTradeOfferRequest struct {
	UID      uint64
	Gold     uint64
	Locked   bool
	ItemList []SDDirectTradeItem
}

func (v SDDirectTradeOfferRequest) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.UID)
	writer.Uint64(v.Gold)
	writer.Bool(v.Locked)
	writer.SizeTag(uint64(len(v.ItemList)))
	for _, item := range v.ItemList {
		if err := item.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDDirectTradeOfferRequest) UnmarshalPortable(reader *PortableReader) error {
	v.UID = reader.Uint64()
	v.Gold = reader.Uint64()
	v.Locked = reader.Bool()
	count := readPortableCount(reader, 1<<20, "direct trade request item")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.ItemList = make([]SDDirectTradeItem, count)
	for i := range v.ItemList {
		if err := v.ItemList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDDirectTradeOffer struct {
	UID       uint64
	Gold      uint64
	Locked    bool
	Confirmed bool
	ItemList  []SDItem
}

func (v SDDirectTradeOffer) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.UID)
	writer.Uint64(v.Gold)
	writer.Bool(v.Locked)
	writer.Bool(v.Confirmed)
	writer.SizeTag(uint64(len(v.ItemList)))
	for _, item := range v.ItemList {
		if err := item.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDDirectTradeOffer) UnmarshalPortable(reader *PortableReader) error {
	v.UID = reader.Uint64()
	v.Gold = reader.Uint64()
	v.Locked = reader.Bool()
	v.Confirmed = reader.Bool()
	count := readPortableCount(reader, 1<<20, "direct trade offer item")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.ItemList = make([]SDItem, count)
	for i := range v.ItemList {
		if err := v.ItemList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

func (v *SDDirectTradeOffer) Clear() { *v = SDDirectTradeOffer{} }

type SDDirectTradeResult struct {
	UID       uint64
	Gold      uint64
	Inventory SDInventory
}

func (v SDDirectTradeResult) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.UID)
	writer.Uint64(v.Gold)
	return v.Inventory.MarshalPortable(writer)
}

func (v *SDDirectTradeResult) UnmarshalPortable(reader *PortableReader) error {
	v.UID = reader.Uint64()
	v.Gold = reader.Uint64()
	return v.Inventory.UnmarshalPortable(reader)
}

type SDEquipWear struct {
	UID    uint64
	WLType int32
	Item   SDItem
}

func (v SDEquipWear) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.UID)
	writer.Int32(v.WLType)
	return v.Item.MarshalPortable(writer)
}

func (v *SDEquipWear) UnmarshalPortable(reader *PortableReader) error {
	v.UID = reader.Uint64()
	v.WLType = reader.Int32()
	return v.Item.UnmarshalPortable(reader)
}

type SDGrabWear struct {
	WLType int32
	Item   SDItem
}

func (v SDGrabWear) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.WLType)
	return v.Item.MarshalPortable(writer)
}

func (v *SDGrabWear) UnmarshalPortable(reader *PortableReader) error {
	v.WLType = reader.Int32()
	return v.Item.UnmarshalPortable(reader)
}

type SDEquipBelt struct {
	Slot int32
	Item SDItem
}

func (v SDEquipBelt) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.Slot)
	return v.Item.MarshalPortable(writer)
}

func (v *SDEquipBelt) UnmarshalPortable(reader *PortableReader) error {
	v.Slot = reader.Int32()
	return v.Item.UnmarshalPortable(reader)
}

type SDGrabBelt struct {
	Slot int32
	Item SDItem
}

func (v SDGrabBelt) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.Slot)
	return v.Item.MarshalPortable(writer)
}

func (v *SDGrabBelt) UnmarshalPortable(reader *PortableReader) error {
	v.Slot = reader.Int32()
	return v.Item.UnmarshalPortable(reader)
}

type SDBuyCost struct {
	Item     SDItem
	CostList []SDCostItem
}

func (v SDBuyCost) MarshalPortable(writer *PortableWriter) error {
	if err := v.Item.MarshalPortable(writer); err != nil {
		return err
	}
	writer.SizeTag(uint64(len(v.CostList)))
	for _, cost := range v.CostList {
		if err := cost.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDBuyCost) UnmarshalPortable(reader *PortableReader) error {
	if err := v.Item.UnmarshalPortable(reader); err != nil {
		return err
	}
	count := readPortableCount(reader, 1<<20, "buy cost")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.CostList = make([]SDCostItem, count)
	for i := range v.CostList {
		if err := v.CostList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDPickUpItemList struct {
	FailedItemID uint32
	ItemList     []SDItem
}

func (v SDPickUpItemList) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.FailedItemID)
	writer.SizeTag(uint64(len(v.ItemList)))
	for _, item := range v.ItemList {
		if err := item.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDPickUpItemList) UnmarshalPortable(reader *PortableReader) error {
	v.FailedItemID = reader.Uint32()
	count := readPortableCount(reader, 1<<20, "pickup item")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.ItemList = make([]SDItem, count)
	for i := range v.ItemList {
		if err := v.ItemList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDStallItem struct {
	Item  SDItem
	Price uint64
}

func (v SDStallItem) MarshalPortable(writer *PortableWriter) error {
	if err := v.Item.MarshalPortable(writer); err != nil {
		return err
	}
	writer.Uint64(v.Price)
	return writer.Err()
}

func (v *SDStallItem) UnmarshalPortable(reader *PortableReader) error {
	if err := v.Item.UnmarshalPortable(reader); err != nil {
		return err
	}
	v.Price = reader.Uint64()
	return reader.Err()
}

type SDStall struct {
	DBID     uint32
	Selling  bool
	Name     string
	ItemList []SDStallItem
}

func (v SDStall) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.DBID)
	writer.Bool(v.Selling)
	writer.String(v.Name)
	writer.SizeTag(uint64(len(v.ItemList)))
	for _, item := range v.ItemList {
		if err := item.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDStall) UnmarshalPortable(reader *PortableReader) error {
	v.DBID = reader.Uint32()
	v.Selling = reader.Bool()
	v.Name = reader.String()
	count := readPortableCount(reader, 1<<20, "stall item")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.ItemList = make([]SDStallItem, count)
	for i := range v.ItemList {
		if err := v.ItemList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

func (v SDStall) Empty() bool { return len(v.ItemList) == 0 }
func (v *SDStall) Clear()     { *v = SDStall{} }

type SDItemStorage struct {
	Gold      uint64
	Wear      SDWear
	Belt      SDBelt
	Inventory SDInventory
}

func (v SDItemStorage) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.Gold)
	if err := v.Wear.MarshalPortable(writer); err != nil {
		return err
	}
	if err := v.Belt.MarshalPortable(writer); err != nil {
		return err
	}
	return v.Inventory.MarshalPortable(writer)
}

func (v *SDItemStorage) UnmarshalPortable(reader *PortableReader) error {
	v.Gold = reader.Uint64()
	if err := v.Wear.UnmarshalPortable(reader); err != nil {
		return err
	}
	if err := v.Belt.UnmarshalPortable(reader); err != nil {
		return err
	}
	return v.Inventory.UnmarshalPortable(reader)
}

func (v *SDItemStorage) Clear() { *v = SDItemStorage{} }

type SDWLDesp struct {
	Wear      SDWear
	Hair      uint32
	HairColor uint32
}

func (v SDWLDesp) MarshalPortable(writer *PortableWriter) error {
	if err := v.Wear.MarshalPortable(writer); err != nil {
		return err
	}
	writer.Uint32(v.Hair)
	writer.Uint32(v.HairColor)
	return writer.Err()
}

func (v *SDWLDesp) UnmarshalPortable(reader *PortableReader) error {
	if err := v.Wear.UnmarshalPortable(reader); err != nil {
		return err
	}
	v.Hair = reader.Uint32()
	v.HairColor = reader.Uint32()
	return reader.Err()
}

func (v *SDWLDesp) Clear() { *v = SDWLDesp{} }

type SDUIDWLDesp struct {
	UID  uint64
	Desp SDWLDesp
}

func (v SDUIDWLDesp) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.UID)
	return v.Desp.MarshalPortable(writer)
}

func (v *SDUIDWLDesp) UnmarshalPortable(reader *PortableReader) error {
	v.UID = reader.Uint64()
	return v.Desp.UnmarshalPortable(reader)
}

type SDStartGameScene struct {
	UID       uint64
	MapUID    uint64
	X         int32
	Y         int32
	Direction int32
	Desp      SDWLDesp
	Name      string
	NameColor uint32
	ViewRange int32
}

func (v SDStartGameScene) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.UID)
	writer.Uint64(v.MapUID)
	writer.Int32(v.X)
	writer.Int32(v.Y)
	writer.Int32(v.Direction)
	if err := v.Desp.MarshalPortable(writer); err != nil {
		return err
	}
	writer.String(v.Name)
	writer.Uint32(v.NameColor)
	writer.Int32(v.ViewRange)
	return writer.Err()
}

func (v *SDStartGameScene) UnmarshalPortable(reader *PortableReader) error {
	v.UID = reader.Uint64()
	v.MapUID = reader.Uint64()
	v.X = reader.Int32()
	v.Y = reader.Int32()
	v.Direction = reader.Int32()
	if err := v.Desp.UnmarshalPortable(reader); err != nil {
		return err
	}
	v.Name = reader.String()
	v.NameColor = reader.Uint32()
	v.ViewRange = reader.Int32()
	return reader.Err()
}
