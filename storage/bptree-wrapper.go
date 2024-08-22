package storage

import (
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

func UpdateBp(rows []interface{}, tableObj TableObj, pageInfObj PageInfo) error {
	for _, row := range rows {
		rowV2 := row.(*RowV2)

		item := Item{
			Key:   rowV2.ID,
			Value: pageInfObj.Offset,
		}

		tableObj.BpTree.ReplaceOrInsert(item)
	}

	items := GetAllItems(tableObj.BpTree)

	itemsBytes, err := EncodeItems(items)
	if err != nil {
		return fmt.Errorf("UpdateBp: %w", err)
	}

	err = WriteNonPageFile(tableObj.BpFile, itemsBytes)
	if err != nil {
		return fmt.Errorf("UpdateBp: %w", err)
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
