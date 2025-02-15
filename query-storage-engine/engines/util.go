package engines

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"runtime/pprof"
	"time"

	"math/rand"
)

const (
	FRONT_SERVER = ":8080"
)

func SendSql(sql string) (interface{}, error) {
	conn, err := net.Dial("tcp", FRONT_SERVER)
	if err != nil {
		return nil, fmt.Errorf("couldn't stablish connection: %s", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte(sql))
	if err != nil {
		return nil, fmt.Errorf("couldn't write message: %s", err)
	}

	var rawData []byte
	for {
		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("error reading response: %s", err)
		}

		rawData = append(rawData, buffer[:n]...)
	}

	var jsonPlan interface{}
	err = json.Unmarshal(rawData, &jsonPlan)
	if err != nil {
		return nil, fmt.Errorf("json encoding failted: %s", err)
	}

	return jsonPlan, nil
}

type Profiler struct {
	cpuFile   *os.File
	startTime time.Time
}

func (p *Profiler) Start(cpuFile string) {
	var err error

	p.cpuFile, err = os.Create(cpuFile)
	if err != nil {
		log.Fatalf("could not create CPU profile: %v", err)
	}

	pprof.StartCPUProfile(p.cpuFile)

	p.startTime = time.Now()
}

func (p *Profiler) Stop() {
	pprof.StopCPUProfile()
	if p.cpuFile != nil {
		p.cpuFile.Close()
	}

	p.writeProfile("heap", "mem.prof")
	p.writeProfile("goroutine", "goroutine.prof")
	p.writeProfile("mutex", "mutex.prof")
	p.writeProfile("block", "block.prof")

	log.Printf("Profiling completed in %v\n", time.Since(p.startTime))
}

func (p *Profiler) writeProfile(profileName, fileName string) {
	profile := pprof.Lookup(profileName)
	if profile == nil {
		log.Printf("Profile %s not available.\n", profileName)
		return
	}

	file, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("could not create %s profile: %v", profileName, err)
	}
	defer file.Close()

	if err := profile.WriteTo(file, 0); err != nil {
		log.Fatalf("could not write %s profile: %v", profileName, err)
	}
}

func GenerateRandomNumber() int {
	randGen := rand.New(rand.NewSource(time.Now().UnixNano()))
	return randGen.Intn(1000) + 1
}

func handleError(err error, msg string) Result {
	return Result{
		Error: err,
		Msg:   msg,
	}
}

func rollbackAndReturn(txId, primary, modifiedColumn string, walManager *WalManager, engine *QueryEngine, catalog *Catalog, err error, msg string) Result {
	if rollbackErr := walManager.AbortTransaction(txId, primary, modifiedColumn, engine, catalog); rollbackErr != nil {
		err = fmt.Errorf("AbortTransaction failed: %w", rollbackErr)
	}
	return Result{
		Error: err,
		Msg:   msg,
	}
}
