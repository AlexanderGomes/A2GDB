package engines

import (
	"a2gdb/logger"
	"fmt"
	"os"
	"path/filepath"
)

type DiskManagerV2 struct {
	DBdirectory string
	PageCatalog *Catalog
	FileCatalog *os.File
	TableObjs   map[string]*TableObj
}

func NewDiskManagerV2(dbDirectory string) (*DiskManagerV2, error) {
	var manager DiskManagerV2

	if _, err := os.Stat(dbDirectory); os.IsNotExist(err) {
		manager, err = CreatDefaultManager(dbDirectory)
		if err != nil {
			logger.Log.Errorf("failed creating default manager: %v", err)
			return nil, fmt.Errorf("NewDiskManagerV2: %w", err)
		}
		logger.Log.Info("Created Default Manager")

	} else {
		manager, err = ReadExistingManager(dbDirectory)
		if err != nil {
			logger.Log.Errorf("failed reading existing manager: %v", err)
			return nil, fmt.Errorf("NewDiskManagerV2: %w", err)
		}
		logger.Log.Info("Loaded Existing Manager")

	}
	return &manager, nil
}

func CreatDefaultManager(dbDirectory string) (DiskManagerV2, error) {
	err := os.Mkdir(dbDirectory, 0755)
	if err != nil {
		return DiskManagerV2{}, fmt.Errorf("CreatDefaultManager (create db Dir error): %w", err)
	}

	err = os.Mkdir(dbDirectory+"/Tables", 0755)
	if err != nil {
		return DiskManagerV2{}, fmt.Errorf("CreatDefaultManager (create table dir error): %w", err)
	}


	catalogFilePtr, err := os.Create(dbDirectory + "/catalog")
	if err != nil {
		return DiskManagerV2{}, fmt.Errorf("CreatDefaultManager (create catalog file error): %w", err)
	}

	catalog := Catalog{Tables: make(map[string]*TableInfo)}
	encodedCatalog, err := SerializeCatalog(&catalog)
	if err != nil {
		return DiskManagerV2{}, fmt.Errorf("CreatDefaultManager: %w", err)
	}

	_, err = catalogFilePtr.Write(encodedCatalog)
	if err != nil {
		return DiskManagerV2{}, fmt.Errorf("CreatDefaultManager (catalog writing error): %w", err)
	}

	dm := DiskManagerV2{
		DBdirectory: dbDirectory,
		PageCatalog: &catalog,
		FileCatalog: catalogFilePtr,
		TableObjs:   make(map[string]*TableObj),
	}

	return dm, nil
}

func ReadExistingManager(dbDirectory string) (DiskManagerV2, error) {
	catalogPath := filepath.Join(dbDirectory, "catalog")

	file, err := os.OpenFile(catalogPath, os.O_RDWR, 0666)
	if err != nil {
		return DiskManagerV2{}, fmt.Errorf("ReadExistingManager: %w", err)
	}

	bytes, err := ReadNonPageFile(file)
	if err != nil {
		return DiskManagerV2{}, fmt.Errorf("ReadExistingManager: %w", err)
	}

	catalog, err := DeserializeCatalog(bytes)
	if err != nil {
		return DiskManagerV2{}, fmt.Errorf("ReadExistingManager: %w", err)
	}

	dm := DiskManagerV2{
		DBdirectory: dbDirectory,
		PageCatalog: catalog,
		FileCatalog: file,
		TableObjs:   make(map[string]*TableObj),
	}

	return dm, nil
}

func UpdateDirectoryPageDisk(page *DirectoryPageV2, dirFile *os.File) error {
	pageBytes, err := EncodeDirectory(page)
	if err != nil {
		return fmt.Errorf("EncodeDirectory failed: %w", err)
	}

	err = WriteNonPageFile(dirFile, pageBytes)
	if err != nil {
		return fmt.Errorf("WriteNonPageFile: %w", err)
	}

	return nil
}
