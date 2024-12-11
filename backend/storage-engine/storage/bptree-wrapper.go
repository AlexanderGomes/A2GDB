package storage

import (
	"fmt"
	"github.com/google/btree"
	"io"
)

type Item struct {
	Key   int64
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

func UpdateBp(rows []int64, tableObj TableObj, pageInfObj PageInfo) error {
	var items []Item

	for _, rowID := range rows {
		item := Item{
			Key:   rowID,
			Value: pageInfObj.Offset,
		}

		items = append(items, item)
		tableObj.BpTree.ReplaceOrInsert(item)
	}

	itemsBytes, err := EncodeItems(items)
	if err != nil {
		return fmt.Errorf("UpdateBp: %w", err)
	}

	_, err = tableObj.BpFile.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("UpdateBp (can't seek to the end): %w", err)
	}

	_, err = tableObj.BpFile.Write(itemsBytes)
	if err != nil {
		return fmt.Errorf("UpdateBp (failed writing bp to disk): %w", err)
	}

	return nil
}

func GetItemByKey(t *btree.BTree, key int64) (*Item, error) {
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
