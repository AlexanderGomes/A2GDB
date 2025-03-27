package engines

import (
	"bytes"
	"reflect"
)

func CreateTuplePools(memCtx *MemoryContext) {
	var rowType = reflect.TypeOf((*RowV2)(nil)).Elem()
	var readerType = reflect.TypeOf((*bytes.Reader)(nil))
	var bufferType = reflect.TypeOf((*bytes.Buffer)(nil))
	var byteSliceType = reflect.TypeOf((*[]byte)(nil))

	capacity := DetermineCapacity(memCtx.allocStrategy)

	memCtx.CreatePool(rowType, RowV2Allocator, RowV2Cleaner, capacity)
	memCtx.CreatePool(readerType, BytesReaderAllocator, BytesReaderCleaner, capacity)
	memCtx.CreatePool(bufferType, BufferAllocator, BufferCleaner, capacity)
	memCtx.CreatePool(byteSliceType, ByteSliceAllocator, ByteSliceCleaner, capacity)
}

func GetTupleObjs(memCtx *MemoryContext) (*RowV2, *bytes.Reader, *bytes.Buffer, *[]byte) {
	rowInterface := memCtx.Acquire(reflect.TypeOf((*RowV2)(nil)).Elem())
	row := rowInterface.(*RowV2)

	readerInterface := memCtx.Acquire(reflect.TypeOf((*bytes.Reader)(nil)))
	reader := readerInterface.(*bytes.Reader)

	bufferInterface := memCtx.Acquire(reflect.TypeOf((*bytes.Buffer)(nil)))
	buffer := bufferInterface.(*bytes.Buffer)

	sliceInterface := memCtx.Acquire(reflect.TypeOf((*[]byte)(nil)))
	slice := sliceInterface.(*[]byte)

	return row, reader, buffer, slice
}

func GetTuplePoolObjs(memCtx *MemoryContext) (*Pool, *Pool, *Pool, *Pool) {
	var rowType = reflect.TypeOf((*RowV2)(nil)).Elem()
	var readerType = reflect.TypeOf((*bytes.Reader)(nil))
	var bufferType = reflect.TypeOf((*bytes.Buffer)(nil))
	var byteSliceType = reflect.TypeOf((*[]byte)(nil))

	rowPoolObj := memCtx.GetPool(rowType)
	readerPoolObj := memCtx.GetPool(readerType)
	bufferPoolObj := memCtx.GetPool(bufferType)
	byteSlicePoolObj := memCtx.GetPool(byteSliceType)

	return rowPoolObj, readerPoolObj, bufferPoolObj, byteSlicePoolObj
}

func ReleaseTupleObjs(memCtx *MemoryContext, row *RowV2, reader *bytes.Reader, buffer *bytes.Buffer, slice *[]byte) {
	var rowType = reflect.TypeOf((*RowV2)(nil)).Elem()
	var readerType = reflect.TypeOf((*bytes.Reader)(nil))
	var bufferType = reflect.TypeOf((*bytes.Buffer)(nil))
	var byteSliceType = reflect.TypeOf((*[]byte)(nil))

	memCtx.Release(rowType, row)
	memCtx.Release(readerType, reader)
	memCtx.Release(bufferType, buffer)
	memCtx.Release(byteSliceType, slice)
}

func CreateAccountingPools(memCtx *MemoryContext) {
	var freeSpaceType = reflect.TypeOf((*FreeSpace)(nil)).Elem()
	var modifiedInfoType = reflect.TypeOf((*ModifiedInfo)(nil)).Elem()
	var nonAddedRowsType = reflect.TypeOf((*NonAddedRows)(nil)).Elem()

	capacity := DetermineCapacity(memCtx.allocStrategy)

	memCtx.CreatePool(freeSpaceType, FreeSpaceAllocator, FreeSpaceCleaner, capacity)
	memCtx.CreatePool(modifiedInfoType, ModifiedInfoAllocator, ModifiedInfoCleaner, capacity)
	memCtx.CreatePool(nonAddedRowsType, NonAddedRowsAllocator, NonAddedRowsCleaner, capacity)
}

func GetAccountingObjs(memCtx *MemoryContext) (*FreeSpace, *ModifiedInfo, *NonAddedRows) {
	freeSpaceInterface := memCtx.Acquire(reflect.TypeOf((*FreeSpace)(nil)).Elem())
	freeSpace := freeSpaceInterface.(*FreeSpace)

	mdInterface := memCtx.Acquire(reflect.TypeOf((*ModifiedInfo)(nil)).Elem())
	modified := mdInterface.(*ModifiedInfo)

	ndInterface := memCtx.Acquire(reflect.TypeOf((*NonAddedRows)(nil)).Elem())
	nonAddedRow := ndInterface.(*NonAddedRows)

	return freeSpace, modified, nonAddedRow
}

func GetAccountingPoolObjs(memCtx *MemoryContext) (*Pool, *Pool, *Pool) {
	var freeSpaceType = reflect.TypeOf((*FreeSpace)(nil)).Elem()
	var modifiedInfoType = reflect.TypeOf((*ModifiedInfo)(nil)).Elem()
	var nonAddedRowsType = reflect.TypeOf((*NonAddedRows)(nil)).Elem()

	freeSpacePoolObj := memCtx.GetPool(freeSpaceType)
	ModifiedInfoPoolObj := memCtx.GetPool(modifiedInfoType)
	nonAddedRowPoolObj := memCtx.GetPool(nonAddedRowsType)

	return freeSpacePoolObj, ModifiedInfoPoolObj, nonAddedRowPoolObj
}

func ReleaseAccountingObjs(memCtx *MemoryContext, freeSpace *FreeSpace, modified *ModifiedInfo, nonAddedRow *NonAddedRows) {
	var freeSpaceType = reflect.TypeOf((*FreeSpace)(nil)).Elem()
	var modifiedInfoType = reflect.TypeOf((*ModifiedInfo)(nil)).Elem()
	var nonAddedRowsType = reflect.TypeOf((*NonAddedRows)(nil)).Elem()

	memCtx.Release(freeSpaceType, freeSpace)
	memCtx.Release(modifiedInfoType, modified)
	memCtx.Release(nonAddedRowsType, nonAddedRow)
}
