package engines

import "bytes"

func RowV2Cleaner(obj any) {
	if row, ok := obj.(*RowV2); ok {
		row.ID = 0
		row.Size = 0

		for k := range row.Values {
			delete(row.Values, k)
		}
	}
}

func BytesReaderCleaner(obj any) {
	if r, ok := obj.(*bytes.Reader); ok {
		r.Reset(nil)
	}
}

func BufferCleaner(obj any) {
	if buf, ok := obj.(*bytes.Buffer); ok {
		buf.Reset()
	}
}

func ByteSliceCleaner(obj any) {
	if slice, ok := obj.(*[]byte); ok {
		*slice = (*slice)[:0]
	}
}

func FreeSpaceCleaner(obj any) {
	if fs, ok := obj.(*FreeSpace); ok {
		fs.PageID = 0
		fs.FreeMemory = 0
		fs.TempPagePtr = nil
	}
}

func ModifiedInfoCleaner(obj any) {
	if mi, ok := obj.(*ModifiedInfo); ok {
		mi.FreeSpaceMapping = nil
		mi.NonAddedRow = nil
	}
}

func NonAddedRowsCleaner(obj any) {
	if nar, ok := obj.(*NonAddedRows); ok {
		nar.BytesNeeded = 0

		for i := range nar.Rows {
			nar.Rows[i] = nil
		}

		nar.Rows = nar.Rows[:0]
	}
}
