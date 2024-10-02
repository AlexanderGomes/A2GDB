package tests

import (
	"disk-db/cmd"
	"testing"
)

const (
	replacerFrequency = 2
	dirName           = "A2G_DB"
)

func BenchmarkInsert(b *testing.B) {
	dm, _ := cmd.InitDatabase(replacerFrequency, dirName)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := dm.QueryEntryPoint(`INSERT INTO User (Username, Age, City) VALUES
            ('sander1', 12, 'richmond'),
            ('sander2', 15, 'richmond'),
            ('sander3', 23, 'richmond'),
            ('sander4', 11, 'richmond'),
            ('sander5', 7, 'richmond');`)
		if err != nil {
			b.Fatalf("Inserting rows failed: %v", err)
		}
	}
}
