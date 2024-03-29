package main

import (
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/partition/gpt"
	toml "github.com/pelletier/go-toml"
)

const MaxBootsectorSize = 440

const (
	PartitionTypeFile = "file"
	PartitionTypeFS   = "fs"
)

type Partition struct {
	ptype   string
	name    string
	size    uint64
	gptType string
	gptUUID string

	file string

	fsType filesystem.Type
	files  []string
}

func cfgParsePartition(index int, partition *toml.Tree) Partition {
	if !partition.Has("type") {
		panic(fmt.Errorf("partition %d: missing type", index))
	}
	cfgType := partition.Get("type").(string)

	if !partition.Has("gpt-type") {
		panic(fmt.Errorf("partition %d: missing gpt-type guid", index))
	}
	cfgGptType := partition.Get("gpt-type").(string)

	var cfgGptUUID string = ""
	if partition.Has("gpt-uuid") {
		cfgGptUUID = partition.Get("gpt-uuid").(string)
	}

	var cfgName string = ""
	if partition.Has("name") {
		cfgName = partition.Get("name").(string)
	}

	commonPart := Partition{
		name:    cfgName,
		gptType: cfgGptType,
		gptUUID: cfgGptUUID,
	}

	switch cfgType {
	case "file":
		if !partition.Has("file") {
			panic(fmt.Errorf("partition %d: no file", index))
		}
		cfgFile := partition.Get("file").(string)
		fileStat, err := os.Stat(cfgFile)
		if err != nil {
			if os.IsNotExist(err) {
				panic(fmt.Errorf("partition %d: file %s does not exist", index, cfgFile))
			}
			panic(err)
		}

		commonPart.ptype = PartitionTypeFile
		commonPart.size = uint64(fileStat.Size())
		commonPart.file = cfgFile
		return commonPart
	case "fs":
		if !partition.Has("size") {
			panic(fmt.Errorf("partition %d: no size", index))
		}
		cfgSize := uint64(partition.Get("size").(int64)) * 1024 * 1024

		if !partition.Has("fs-type") {
			panic(fmt.Errorf("partition %d: no fs-type", index))
		}
		cfgFsType := partition.Get("fs-type").(string)

		cfgFiles := make([]string, 0)
		if partition.Has("files") {
			cfgFiles = partition.GetArray("files").([]string)
		}

		switch cfgFsType {
		case "fat32":
			commonPart.fsType = filesystem.TypeFat32
		default:
			panic(fmt.Errorf("partition %d: invalid fs-type \"%s\"", index, cfgFsType))
		}

		commonPart.files = cfgFiles
		commonPart.ptype = PartitionTypeFS
		commonPart.size = cfgSize
		return commonPart
	default:
		panic(fmt.Errorf("partition %d: invalid type \"%s\"", index, cfgType))
	}
}

func main() {
	configPath := flag.String("config", "mkimg.toml", "mkimg configuration")
	flag.Parse()

	data, err := os.ReadFile(*configPath)
	if err != nil {
		panic(err)
	}

	config, err := toml.Load(string(data))
	if err != nil {
		panic(err)
	}

	cfgName := "out.img"
	if config.Has("name") {
		cfgName = config.Get("name").(string)
	}

	var cfgFirstSector uint64 = 2048
	if config.Has("first-sector") {
		cfgFirstSector = uint64(config.Get("first-sector").(int64))
	}

	cfgBootsector := ""
	if config.Has("bootsector") {
		cfgBootsector = config.Get("bootsector").(string)
	}

	// Remove the image
	if err := os.Remove(cfgName); err != nil && !os.IsNotExist(err) {
		panic(err)
	}

	// Parse config for partitions
	cfgPartitions := config.GetArray("partitions").([]*toml.Tree)
	if cfgPartitions == nil {
		panic("no partitions")
	}

	var size uint64 = 512 * cfgFirstSector * 2
	partitions := make([]Partition, 0)
	for i, cfgPartition := range cfgPartitions {
		partition := cfgParsePartition(i+1, cfgPartition)
		size += (partition.size + 512 - 1) / 512 * 512
		partitions = append(partitions, partition)
	}

	// Create the image
	fmt.Printf("Creating image %s\n", cfgName)
	img, err := diskfs.Create(cfgName, int64(size), diskfs.Raw, diskfs.SectorSize512)
	if err != nil {
		panic(err)
	}

	currentSector := cfgFirstSector
	gptPartitions := make([]*gpt.Partition, 0)
	for i, partition := range partitions {
		sizeInSectors := uint64((int64(partition.size) + img.LogicalBlocksize - 1) / img.LogicalBlocksize)
		gptPartition := gpt.Partition{
			Start: currentSector,
			End:   currentSector + sizeInSectors,
			Type:  gpt.Type(partition.gptType),
			Name:  partition.name,
		}
		if partition.gptUUID != "" {
			gptPartition.GUID = partition.gptUUID
		}
		gptPartitions = append(gptPartitions, &gptPartition)
		fmt.Printf("> Partition %d { name: %s, type: %s, start: %d, end: %d, GUID: %s }\n", i+1, partition.name, partition.ptype, currentSector, currentSector+sizeInSectors, partition.gptUUID)
		currentSector += sizeInSectors
	}

	// Partition the image
	protectiveMBR := false
	if config.Has("protective-mbr") {
		protectiveMBR = config.Get("protective-mbr").(bool)
	}
	if err := img.Partition(&gpt.Table{
		Partitions:    gptPartitions,
		ProtectiveMBR: protectiveMBR,
	}); err != nil {
		panic(err)
	}

	// Fulfill the partitions
	for i, partition := range partitions {
		switch partition.ptype {
		case PartitionTypeFile:
			file, err := os.Open(partition.file)
			if err != nil {
				panic(err)
			}

			img.WritePartitionContents(i+1, file)
		case PartitionTypeFS:
			fs, err := img.CreateFilesystem(disk.FilesystemSpec{Partition: i + 1, FSType: partition.fsType, VolumeLabel: partition.name})
			if err != nil {
				panic(err)
			}

			var fsCopy func(srcPath string, isDir bool, destPath string)
			fsCopy = func(srcPath string, isDir bool, destPath string) {
				if isDir {
					files, err := os.ReadDir(srcPath)
					if err != nil {
						panic(err)
					}

					for _, file := range files {
						fs.Mkdir(destPath)
						fsCopy(path.Join(srcPath, file.Name()), file.IsDir(), path.Join(destPath, file.Name()))
					}
				} else {
					fileData, err := os.ReadFile(srcPath)
					if err != nil {
						panic(err)
					}
					file, err := fs.OpenFile(destPath, os.O_CREATE|os.O_RDWR)
					if err != nil {
						panic(err)
					}
					if _, err := file.Write(fileData); err != nil {
						panic(err)
					}
				}
			}

			for _, file := range partition.files {
				stat, err := os.Stat(file)
				if err != nil {
					panic(err)
				}
				destPath := "/"
				if !stat.IsDir() {
					destPath = path.Join(destPath, stat.Name())
				}
				fsCopy(file, stat.IsDir(), destPath)
			}
		}
	}

	// Write the bootsector
	if cfgBootsector != "" {
		bootsector, err := os.ReadFile(cfgBootsector)
		if err != nil {
			panic(err)
		}

		bootsectorSize := int64(len(bootsector))
		if bootsectorSize > MaxBootsectorSize {
			panic(fmt.Errorf("bootsector exceeds maximum size of 440 bytes (%d bytes)", bootsectorSize))
		}
		if _, err = img.File.WriteAt(bootsector, 0); err != nil {
			panic(err)
		}
		fmt.Printf("> Bootsector { size: %d }\n", bootsectorSize)
	}
	fmt.Println("Done")
}
