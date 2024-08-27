package storage

import "fmt"

type RearrangedPage struct {
	PageID PageID
	Offset Offset
	Size   uint16
}

func (page *PageV2) RearrangePAGE(tableObj *TableObj) error {
	newPage := CreatePageV2()
	newPage.Header.ID = page.Header.ID

	pageObj, ok := tableObj.DirectoryPage.Value[PageID(page.Header.ID)]
	if !ok {
		return fmt.Errorf("RearrangePAGE :pageObj not found")
	}

	for _, location := range pageObj.PointerArray {
		rowBytes := page.Data[location.Offset : location.Offset+location.Length]

		err := newPage.AddTuple(rowBytes)
		if err != nil {
			return fmt.Errorf("RearrangePAGE: %w", err)
		}
	}

	pageObj.Rearranged = true
	pageObj.PointerArray = page.PointerArray
	pageObj.Size = newPage.Header.UpperPtr - newPage.Header.LowerPtr

	err := WritePageBackV2(newPage, pageObj.Offset, tableObj.DataFile)
	if err != nil {
		return fmt.Errorf("RearrangePAGE: %w", err)
	}

	err = UpdateDirectoryPageDisk(tableObj.DirectoryPage, tableObj.DirFile)
	if err != nil {
		return fmt.Errorf("RearrangePAGE: %w", err)
	}

	return nil
}
