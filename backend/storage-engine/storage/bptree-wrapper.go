package storage

import (
	"a2gdb/logger"
	"fmt"

	"github.com/google/btree"
)

type Item struct {
	Key   uint64
	Value Offset
}

func (i Item) Less(other btree.Item) bool {
	return i.Key < other.(Item).Key
}

func GetAllItems(t *btree.BTree) []Item {
	var items []Item
	t.Ascend(func(item btree.Item) bool {
		items = append(items, item.(Item))
		return true
	})
	return items
}

func NewTree(degree int) *btree.BTree {
	return btree.New(degree)
}

func UpdateBp(rows []uint64, tableObj TableObj, pageInfObj PageInfo) error {
	if rows == nil {
		return nil
	}

	var items []Item

	// deleting
	if rows[0] == 0 {
		for _, rowID := range rows {
			item := Item{
				Key:   rowID,
				Value: pageInfObj.Offset,
			}

			logger.Log.WithField("item", item).Info("Deleting From Bptree")
			tableObj.BpTree.Delete(item)
		}

		items = GetAllItems(tableObj.BpTree)
	} else {
		for _, rowID := range rows {
			item := Item{
				Key:   rowID,
				Value: pageInfObj.Offset,
			}

			logger.Log.WithField("item", item).Info("Inserting Into Bptree")
			items = append(items, item)
			tableObj.BpTree.ReplaceOrInsert(item)
		}
	}

	itemsBytes, err := EncodeItems(items)
	if err != nil {
		return fmt.Errorf("updateBp: %w", err)
	}

	err = WriteNonPageFile(tableObj.BpFile, itemsBytes)
	if err != nil {
		return fmt.Errorf("saving bp to disk failed: %w", err)
	}

	return nil
}

func GetItemByKey(t *btree.BTree, key uint64) (*Item, error) {
	searchItem := Item{Key: key}

	item := t.Get(searchItem)
	if item == nil {
		return nil, fmt.Errorf("item with key %d not found", key)
	}

	resultItem, ok := item.(Item)
	if !ok {
		return nil, fmt.Errorf("type assertion failed")
	}

	return &resultItem, nil
}
