package engines

import "bytes"

const (
	SLICE_CAPACITY = 512
)

func RowV2Allocator() any {
	return &RowV2{
		Values: make(map[string]string),
		ID:     GenerateRandomID(),
	}
}

func BytesReaderAllocator() any {
	return &bytes.Reader{}
}

func BufferAllocator() any {
	return &bytes.Buffer{}
}

func ByteSliceAllocator() any {
	s := make([]byte, 0, SLICE_CAPACITY)
	return &s
}

func FreeSpaceAllocator() any {
	return &FreeSpace{}
}

func ModifiedInfoAllocator() any {
	return &ModifiedInfo{}
}

func NonAddedRowsAllocator() any {
	return &NonAddedRows{}
}
