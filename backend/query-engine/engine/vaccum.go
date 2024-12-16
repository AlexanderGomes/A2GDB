package engine

import (
	"a2gdb/storage-engine/storage"
	"log"
)

const (
	ALMOST_FULL_PAGE = 486
	FIRST_LEVEL      = 886
	SECOND_LEVEL     = 1286
	THIRD_LEVEL      = 1686
	FOURTH_LEVEL     = 2086
	FIFITH_LEVEL     = 2486
	SIXTH_LEVEL      = 2886
	SEVENTH_LEVEL    = 3286
	EIGHT_LEVEL      = 3686
	EMPTY_PAGE       = 4086
)

func (qe *QueryEngine) vaccumEntry(newSpace []*storage.FreeSpace, tableObj *storage.TableObj) {

	for _, space := range newSpace {
		switch {
		case space.FreeMemory <= ALMOST_FULL_PAGE:
			tableObj.Memory[ALMOST_FULL_PAGE] = append(tableObj.Memory[ALMOST_FULL_PAGE], space)
		case space.FreeMemory <= FIRST_LEVEL:
			tableObj.Memory[FIRST_LEVEL] = append(tableObj.Memory[FIRST_LEVEL], space)
		case space.FreeMemory <= SECOND_LEVEL:
			tableObj.Memory[SECOND_LEVEL] = append(tableObj.Memory[SECOND_LEVEL], space)
		case space.FreeMemory <= THIRD_LEVEL:
			tableObj.Memory[THIRD_LEVEL] = append(tableObj.Memory[THIRD_LEVEL], space)
		case space.FreeMemory <= FOURTH_LEVEL:
			tableObj.Memory[FOURTH_LEVEL] = append(tableObj.Memory[FOURTH_LEVEL], space)
		case space.FreeMemory <= FIFITH_LEVEL:
			tableObj.Memory[FIFITH_LEVEL] = append(tableObj.Memory[FIFITH_LEVEL], space)
		case space.FreeMemory <= SIXTH_LEVEL:
			tableObj.Memory[SIXTH_LEVEL] = append(tableObj.Memory[SIXTH_LEVEL], space)
		case space.FreeMemory <= SEVENTH_LEVEL:
			tableObj.Memory[SEVENTH_LEVEL] = append(tableObj.Memory[SEVENTH_LEVEL], space)
		case space.FreeMemory <= EIGHT_LEVEL:
			tableObj.Memory[EIGHT_LEVEL] = append(tableObj.Memory[EIGHT_LEVEL], space)
		case space.FreeMemory <= EMPTY_PAGE:
			tableObj.Memory[EMPTY_PAGE] = append(tableObj.Memory[EMPTY_PAGE], space)
		}
	}

	bytes, err := storage.EncodeMemObj(tableObj.Memory)
	if err != nil {
		log.Fatalf("encoding mem obj failed: %s", err)
	}

	err = storage.WriteNonPageFile(tableObj.MemFile, bytes)
	if err != nil {
		log.Fatalf("writing mem obj failed: %s", err)
	}
}
