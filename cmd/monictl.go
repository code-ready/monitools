package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/code-ready/monitools/tools" // local tools package
)

func main() {

	// where to log
	t := time.Now()
	timestamp := t.Format("20060102150405")
	if err := os.MkdirAll("logs", 0766); err != nil {
		log.Fatal("Unable to create logs directory")
	}
	logFilePath := filepath.Join("logs", "monitools_"+timestamp+".log")
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		fmt.Printf("Could not open a file for logging: %v", err)
	}
	log.SetOutput(logFile)

	// set up data folder
	dirName := fmt.Sprintf("data_%s", time.Now().Format("2006-01-02"))
	defaultDir := filepath.Join("data", dirName) // data/data_<date>

	// Command line flags
	var dirPath string
	flag.StringVar(&dirPath, "d", defaultDir, "destination directory")
	var numRepeats int
	flag.IntVar(&numRepeats, "n", 5, "number of checks of CPU load")
	var sleepLength int
	flag.IntVar(&sleepLength, "s", 1, "sleep between repeats [in seconds]")
	var repeatStarts bool
	flag.BoolVar(&repeatStarts, "r", false, "repeatedly start and delete the cluster")
	var pullSecretPath string
	flag.StringVar(&pullSecretPath, "p", "", "path to pull secret file [needed if -r flag is set to true")
	var bundlePath string
	flag.StringVar(&bundlePath, "b", "", "path to CRC bundle [needed if -r flag is set to true")

	flag.Parse()

	// Local information
	//
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		log.Fatalf("Unable to create directory: %s", dirPath)
	}

	// Require running cluster if not doing start/delete testing
	if !repeatStarts && !tools.IsCRCRunning() {
		fmt.Println("CRC VM is not running")
		os.Exit(1)
	}

	// Let the user know about the settings they're using
	fmt.Println("-------------")
	fmt.Println("Running monitoring tools with the following settings:")
	fmt.Printf("Data directory: %s\n", dirPath)
	fmt.Printf("Number of repeats: %d\n", numRepeats)
	if !repeatStarts {
		fmt.Printf("Pauses between repeats: %ds\n", sleepLength)
	}
	fmt.Printf("Logging into: %s\n", logFilePath)
	fmt.Println("-------------")

	cpuChan := make(chan error)
	trafficChan := make(chan error)
	crioChan := make(chan error)
	nodeDesChan := make(chan error)
	startChan := make(chan error)

	// ================
	// start collecting
	// ================

	if repeatStarts {
		// start times for CRC cluster
		startTimesFile := filepath.Join(dirPath, "startTimes.json")
		go tools.RecordStartTimes(startTimesFile, numRepeats, startChan, pullSecretPath, bundlePath)
		log.Println("going to record start times for CRC cluster")
	} else {
		// transmitted/received MiB on crc interface
		trafficFile := filepath.Join(dirPath, "traffic.json")
		go tools.RecordTraffic(trafficFile, numRepeats, sleepLength, trafficChan)
		log.Println("going to record traffic going in/out of the VM")

		// CPU usage by 'qemu' process
		cpuFile := filepath.Join(dirPath, "cpu.json")
		go tools.RecordHostCPUUsage(cpuFile, numRepeats, sleepLength, cpuChan)
		log.Println("going to record CPU usage percentage attributed to qemu")

		// CRI-O stats as reported by 'crictl'
		go tools.GetCRIStatsFromVM(dirPath, crioChan)
		log.Println("going to retrieve crictl stats from the CRC VM")

		// Node Description
		nodeDescription := filepath.Join(dirPath, "node.json")
		go tools.GetNodeResource(nodeDescription, nodeDesChan)
	}
	// ================
	// done collecting
	// ================

	if repeatStarts {
		if err := <-startChan; err != nil {
			log.Fatalf("failed to record start times: %s", err)
		} else {
			log.Printf("recorded start duration %d times", numRepeats)
		}
	} else {
		if err := <-trafficChan; err != nil {
			log.Fatalf("failed to record traffic flow %s", err)
		} else {
			log.Printf("recorded traffic (RX/TX) %d times at %d sec intervals", numRepeats, sleepLength)
		}

		if err := <-cpuChan; err != nil {
			log.Fatalf("failed to record CPU percentage %s", err)
		} else {
			log.Printf("recorded CPU usage percentage %d times at %d sec intervals", numRepeats, sleepLength)
		}

		if err := <-crioChan; err != nil {
			log.Fatalf("could not retrieve crictl stats: %s", err)
		} else {
			log.Println("crictl stats successfully retrieved")
		}

		if err := <-nodeDesChan; err != nil {
			log.Fatalf("could not retrieve node description stats: %s", err)
		} else {
			log.Println("node description successfully retrieved")
		}
	}
}
