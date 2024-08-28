package storage

import "fmt"

type RearrangedPage struct {
	PageID PageID
	Offset Offset
	Size   uint16
}

func (page *PageV2) RearrangePAGE(tableObj *TableObj) (*PageV2, error) {
	newPage := CreatePageV2()
	newPage.Header.ID = page.Header.ID

	pageObj, ok := tableObj.DirectoryPage.Value[PageID(page.Header.ID)]
	if !ok {
		return nil, fmt.Errorf("RearrangePAGE :pageObj not found")
	}

	for _, location := range pageObj.PointerArray {
		if !location.Free {
			rowBytes := page.Data[location.Offset : location.Offset+location.Length]

			err := newPage.AddTuple(rowBytes)
			if err != nil {
				return nil, fmt.Errorf("RearrangePAGE: %w", err)
			}
		}
	}

	pageObj.Rearranged = true
	pageObj.PointerArray = page.PointerArray
	pageObj.Size = newPage.Header.UpperPtr - newPage.Header.LowerPtr

	rearranged := &RearrangedPage{
		PageID: PageID(page.Header.ID),
		Offset: pageObj.Offset,
		Size:   pageObj.Size,
	}

	tableObj.RearrangedPages = append(tableObj.RearrangedPages, rearranged)

	return newPage, nil
}
