package engine

import (
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
