package main

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"unsafe"

	"github.com/urfave/cli/v2"
)

//go:generate stringer -type=RelationType
type RelationType int

const (
	RelationProcessorCore RelationType = iota
	RelationNumaNode
	RelationCache
	RelationProcessorPackage
	RelationGroup
	RelationProcessorDie
	RelationNumaNodeEx
	RelationProcessorModule
	RelationAll = 0xffff
)

type SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX struct {
	Relationship uint32
	Size         uint32
	data         interface{}
}

type PROCESSOR_RELATIONSHIP struct {
	Flags           byte
	EfficiencyClass byte
	Reserved        [20]byte
	GroupCount      uint16
	GroupMasks      interface{} //[]GROUP_AFFINITY // in c++ this is a union of either one or many GROUP_AFFINITY based on GroupCount
}

type GROUP_AFFINITY struct {
	Mask     uintptr
	Group    uint16
	Reserved [3]uint16
}

type NUMA_NODE_RELATIONSHIP struct {
	NodeNumber uint32
	Reserved   [18]byte
	GroupCount uint16
	GroupMasks interface{} //[]GROUP_AFFINITY // in c++ this is a union of either one or many GROUP_AFFINITY based on GroupCount
}

type CACHE_RELATIONSHIP struct {
	Level         byte
	Associativity byte
	LineSize      uint16
	CacheSize     uint32
	Type          PROCESSOR_CACHE_TYPE
	Reserved      [18]byte
	GroupCount    uint16
	GroupMasks    interface{} //interface{}[]GROUP_AFFINITY // in c++ this is a union of either one or many GROUP_AFFINITY based on GroupCount
}

type PROCESSOR_CACHE_TYPE int

const (
	CacheUnified PROCESSOR_CACHE_TYPE = iota
	CacheInstruction
	CacheData
	CacheTrace
	CacheUnknown
)

type GROUP_RELATIONSHIP struct {
	MaximumGroupCount uint16
	ActiveGroupCount  uint16
	Reserved          [20]byte
	GroupInfo         interface{} //[]PROCESSOR_GROUP_INFO
}

type PROCESSOR_GROUP_INFO struct {
	MaximumProcessorCount byte
	ActiveProcessorCount  byte
	Reserved              [38]byte
	ActiveProcessorMask   uintptr
}

var (
	modkernel32                          = syscall.NewLazyDLL("kernel32.dll")
	procGetLogicalProcessorInformationEx = modkernel32.NewProc("GetLogicalProcessorInformationEx")
)

func main() {
	//parameter := os.Args[1]
	fmt.Print("Welcome to (go)core\n\n") //, parameter)

	app := &cli.App{
		Usage: "get processor information",
		Commands: []*cli.Command{
			{
				Name: "info",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "relationship",
						Usage:   "return all processor information",
						Aliases: []string{"r"},
					},
				},
				Action: func(c *cli.Context) error {
					parameter := c.String("relationship")

					var relationship RelationType
					switch parameter {
					case "processor":
						relationship = RelationProcessorCore
					case "numa":
						relationship = RelationNumaNode
					case "numaex":
						relationship = RelationNumaNodeEx
					case "cache":
						relationship = RelationCache
					case "package":
						relationship = RelationProcessorPackage
					case "group":
						relationship = RelationGroup
					case "die":
						relationship = RelationProcessorDie
					default:
						relationship = RelationAll
					}

					fmt.Printf("relationship type: %s\n\n", relationship.String())
					processorInfo(relationship)
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}

	println("\nhave a (go)core day!")
}

func PrintMask(mask uintptr) int {
	count := 0
	for i := 0; i < 64; i++ {
		if mask&(1<<i) != 0 {
			count++
			fmt.Printf("Logical Processors: %d\n", i)
		}
	}
	return count
}

func processorInfo(relationShip RelationType) {
	// Call once to get the length of data to return
	var returnLength uint32 = 0
	r1, _, err := procGetLogicalProcessorInformationEx.Call(
		uintptr(relationShip),
		uintptr(0),
		uintptr(unsafe.Pointer(&returnLength)),
	)
	if r1 != 0 && err.(syscall.Errno) != syscall.ERROR_INSUFFICIENT_BUFFER {
		fmt.Println("Call to GetLogicalProcessorInformationEx failed:", err)
		os.Exit(2)
	}

	// Allocate the buffer with the length it should be
	buffer := make([]byte, returnLength)

	// Call GetLogicalProcessorInformationEx again to get the actual information
	r1, _, err = procGetLogicalProcessorInformationEx.Call(
		uintptr(relationShip),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&returnLength)),
	)
	if r1 == 0 {
		fmt.Println("Call to GetLogicalProcessorInformationEx failed:", err)
		os.Exit(2)
	}

	numOfGroups := 0
	numofSockets := 0
	numOfcores := 0
	numOfLogicalProcessors := 0
	//iterate over the buffer casting it to the correct type
	for offset := 0; offset < len(buffer); {
		info := (*SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX)(unsafe.Pointer(&buffer[offset]))
		switch (RelationType)(info.Relationship) {
		case RelationProcessorCore, RelationProcessorPackage, RelationProcessorDie, RelationProcessorModule:
			processorRelationship := (*PROCESSOR_RELATIONSHIP)(unsafe.Pointer(&info.data))

			groupMasks := make([]GROUP_AFFINITY, processorRelationship.GroupCount)
			for i := 0; i < int(processorRelationship.GroupCount); i++ {
				groupMasks[i] = *(*GROUP_AFFINITY)(unsafe.Pointer(uintptr(unsafe.Pointer(&processorRelationship.GroupMasks)) + uintptr(i)*unsafe.Sizeof(GROUP_AFFINITY{})))
			}

			// todo process properly when using all
			processorCount := 0
			for i := 0; i < int(processorRelationship.GroupCount); i++ {
				fmt.Printf("Core %d(#%d)\n", numOfcores, groupMasks[i].Mask)
				fmt.Printf("mask %064b\n", groupMasks[i].Mask)
				fmt.Printf("Group: %d\n", groupMasks[i].Group)
				processorCount = PrintMask(groupMasks[i].Mask)
			}
			if RelationProcessorCore == (RelationType)(info.Relationship) {
				numOfcores++

			}
			if RelationProcessorPackage == (RelationType)(info.Relationship) {
				numofSockets++
				numOfLogicalProcessors += processorCount
			}
			println()

		case RelationNumaNode, RelationNumaNodeEx:
			numaNodeRelationship := (*NUMA_NODE_RELATIONSHIP)(unsafe.Pointer(&info.data))
			fmt.Printf("Numa Node #%d\n", numaNodeRelationship.NodeNumber)

			groupMasks := make([]GROUP_AFFINITY, numaNodeRelationship.GroupCount)
			for i := 0; i < int(numaNodeRelationship.GroupCount); i++ {
				groupMasks[i] = *(*GROUP_AFFINITY)(unsafe.Pointer(uintptr(unsafe.Pointer(&numaNodeRelationship.GroupMasks)) + uintptr(i)*unsafe.Sizeof(GROUP_AFFINITY{})))
			}

			for i := 0; i < int(numaNodeRelationship.GroupCount); i++ {
				fmt.Printf("mask %064b\n", groupMasks[i].Mask)
				PrintMask(groupMasks[i].Mask)
			}

			println()

		case RelationCache:
			cacheRelationship := (*CACHE_RELATIONSHIP)(unsafe.Pointer(&info.data))
			fmt.Printf("Cache level %d\n", cacheRelationship.Level)
			// TODO Process cache relationship data
			println()

		case RelationGroup:
			groupRelationship := (*GROUP_RELATIONSHIP)(unsafe.Pointer(&info.data))
			numOfGroups = int(groupRelationship.ActiveGroupCount)
			fmt.Printf("Max groups %d\n", groupRelationship.MaximumGroupCount)

			groupInfo := make([]PROCESSOR_GROUP_INFO, groupRelationship.ActiveGroupCount)

			for i := 0; i < int(groupRelationship.ActiveGroupCount); i++ {
				groupInfo[i] = *(*PROCESSOR_GROUP_INFO)(unsafe.Pointer(uintptr(unsafe.Pointer(&groupRelationship.GroupInfo)) + uintptr(i)*unsafe.Sizeof(PROCESSOR_GROUP_INFO{})))
			}

			for i := 0; i < int(groupRelationship.ActiveGroupCount); i++ {
				fmt.Printf("Group %d\n", i)
				fmt.Printf("Max Processors: %d\n", groupInfo[i].MaximumProcessorCount)
				fmt.Printf("Active Processors: %d\n", groupInfo[i].ActiveProcessorCount)
				fmt.Printf("Active Processor Mask: %064b\n", groupInfo[i].ActiveProcessorMask)
				PrintMask(groupInfo[i].ActiveProcessorMask)
				println()
			}

			println()
		}

		offset += int(info.Size)
	}

	fmt.Printf("Number of sockets: %d\n", numofSockets)
	fmt.Printf("Number of cores: %d\n", numOfcores)
	fmt.Printf("Number of logical processors: %d\n", numOfLogicalProcessors)
	fmt.Printf("Number of groups: %d\n", numOfGroups)
}
