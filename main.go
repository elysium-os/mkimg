package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/partition/gpt"
	"github.com/urfave/cli/v3"
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

	// File Partition
	file string

	// FS Partition
	fsType  filesystem.Type
	fsRoot  string
	fsFiles map[string]string
}

// mkimg --partition=fs:name=xdd:type=fat32:size=32mb

func parsePartitionKV(str string) (map[string]string, error) {
	kv := make(map[string]string, 0)

	entries := strings.SplitSeq(str, ":")
	for entry := range entries {
		parts := strings.Split(entry, "=")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid keyvalue `%s` in partition `%s`", entry, str)
		}

		if _, ok := kv[parts[0]]; ok {
			return nil, fmt.Errorf("duplicate partition entry `%s` in `%s`", parts[0], str)
		}

		kv[parts[0]] = parts[1]
	}

	return kv, nil
}

func parsePartition(str string) (Partition, error) {
	kv, err := parsePartitionKV(str)
	if err != nil {
		return Partition{}, err
	}

	partition := Partition{
		name: "Unnamed Partition",
	}

	for k, v := range kv {
		switch k {
		case "type":
			if !slices.Contains([]string{PartitionTypeFile, PartitionTypeFS}, v) {
				return partition, fmt.Errorf("unknown partition type `%s` in partition `%s`", v, str)
			}
			partition.ptype = v
		case "gpt-type":
			partition.gptType = v
		case "name":
			partition.name = v
		default:
			continue
		}
		delete(kv, k)
	}

	if len(partition.ptype) == 0 {
		return partition, fmt.Errorf("partition `%s` is missing a type", str)
	}

	if len(partition.gptType) == 0 {
		return partition, fmt.Errorf("partition `%s` is missing a gpt-type", str)
	}

	switch partition.ptype {
	case PartitionTypeFile:
		for k, v := range kv {
			switch k {
			case "file":
				partition.file = v

				fileStat, err := os.Stat(v)
				if err != nil {
					if os.IsNotExist(err) {
						return partition, fmt.Errorf("file `%s` does not exist (partition `%s`)", v, str)
					}
					panic(err)
				}
				partition.size = uint64(fileStat.Size())
			default:
				continue
			}
			delete(kv, k)
		}
	case PartitionTypeFS:
		for k, v := range kv {
			switch k {
			case "fs-type":
				switch v {
				case "fat32":
					partition.fsType = filesystem.TypeFat32
				default:
					return partition, fmt.Errorf("unknown fs-type `%s` in partition `%s`", v, str)
				}
			case "fs-size":
				size, err := strconv.Atoi(v)
				if err != nil {
					return partition, fmt.Errorf("fs-size `%s` is not a valid number (partition `%s`)", v, str)
				}
				partition.size = uint64(size) * 1024 * 1024
			case "fs-root":
				partition.fsRoot = v
			case "fs-files":
				partition.fsFiles = make(map[string]string, 0)
				for file := range strings.SplitSeq(v, "#") {
					parts := strings.Split(file, "@")
					if len(parts) == 2 {
						partition.fsFiles[parts[0]] = parts[1]
						continue
					}
					partition.fsFiles[parts[0]] = ""
				}
			default:
				continue
			}
			delete(kv, k)
		}
	}

	for k, _ := range kv {
		return partition, fmt.Errorf("unknown partition entry `%s` in `%s`", k, str)
	}

	return partition, nil
}

func createDisk(context context.Context, cmd *cli.Command) error {
	// Read configuration
	name := cmd.String("name")

	// Read partitions
	firstSector := uint64(cmd.Uint("first-sector"))
	size := 512 * firstSector * 2
	partitions := make([]Partition, 0)
	for _, partitionStr := range cmd.StringSlice("partition") {
		partition, err := parsePartition(partitionStr)
		if err != nil {
			return err
		}

		size += (partition.size + 512 - 1) / 512 * 512
		partitions = append(partitions, partition)
	}

	// Create disk image
	fmt.Printf("Creating image %s\n", name)
	if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing image: %s", err)
	}

	img, err := diskfs.Create(name, int64(size), diskfs.SectorSize512)
	if err != nil {
		return fmt.Errorf("failed to create the image: %s", err)
	}

	currentSector := firstSector
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
	if err := img.Partition(&gpt.Table{
		Partitions:    gptPartitions,
		ProtectiveMBR: cmd.Bool("protective-mbr"),
	}); err != nil {
		return fmt.Errorf("failed to partition the image: %s", err)
	}

	// Fulfill the partitions
	for i, partition := range partitions {
		switch partition.ptype {
		case PartitionTypeFile:
			file, err := os.Open(partition.file)
			if err != nil {
				return fmt.Errorf("failed to open `%s`: %s", partition.file, err)
			}

			img.WritePartitionContents(i+1, file)
		case PartitionTypeFS:
			fs, err := img.CreateFilesystem(disk.FilesystemSpec{Partition: i + 1, FSType: partition.fsType, VolumeLabel: partition.name})
			if err != nil {
				return fmt.Errorf("failed to create fs for partition `%s`: %s", partition.name, err)
			}

			var fsCopy func(from string, to string, isDir bool) error
			fsCopy = func(from string, to string, isDir bool) error {
				if isDir {
					files, err := os.ReadDir(from)
					if err != nil {
						return err
					}

					if err := fs.Mkdir(from); err != nil {
						return err
					}

					for _, file := range files {
						if err := fsCopy(path.Join(from, file.Name()), path.Join(to, file.Name()), file.IsDir()); err != nil {
							return err
						}
					}

					return nil
				}

				fileData, err := os.ReadFile(from)
				if err != nil {
					return err
				}

				file, err := fs.OpenFile(to, os.O_CREATE|os.O_RDWR)
				if err != nil {
					return err
				}

				if _, err := file.Write(fileData); err != nil {
					return err
				}

				return nil
			}

			var fsCreateSkeleton func(path string) error
			fsCreateSkeleton = func(path string) error {
				if path == "/" || path == "." {
					return nil
				}

				if err := fsCreateSkeleton(filepath.Dir(path)); err != nil {
					return err
				}

				return fs.Mkdir(path)
			}

			if partition.fsRoot != "" {
				stat, err := os.Stat(partition.fsRoot)
				if err != nil {
					return fmt.Errorf("failed to stat fsroot `%s`: %s", partition.fsRoot, err)
				}

				if !stat.IsDir() {
					return fmt.Errorf("fsroot for partition `%s` it not a directory", partition.name)
				}

				if err := fsCopy(partition.fsRoot, "/", true); err != nil {
					return fmt.Errorf("failed to fsCopy fsroot: %s", err)
				}
			}

			if partition.fsFiles != nil {
				for from, to := range partition.fsFiles {
					stat, err := os.Stat(from)
					if err != nil {
						return fmt.Errorf("failed to stat file `%s`: %s", from, err)
					}

					if to == "" {
						to = stat.Name()
					} else {
						fsCreateSkeleton(filepath.Dir(to))
					}
					to = path.Join("/", to)

					if err := fsCopy(from, to, stat.IsDir()); err != nil {
						return fmt.Errorf("failed to fsCopy `%s` to `%s`: %s", from, to, err)
					}
				}
			}
		}
	}

	// Write the bootsector
	bootsector := cmd.String("bootsector")
	if bootsector != "" {
		bootsector, err := os.ReadFile(bootsector)
		if err != nil {
			return fmt.Errorf("failed to read the bootsector `%s`: %s", bootsector, err)
		}

		bootsectorSize := int64(len(bootsector))
		if bootsectorSize > MaxBootsectorSize {
			return fmt.Errorf("bootsector exceeds maximum size of 440 bytes (%d bytes)", bootsectorSize)
		}

		w, err := img.Backend.Writable()
		if err != nil {
			return fmt.Errorf("failed to open image for writing the bootsector: %s", err)
		}

		if _, err = w.WriteAt(bootsector, 0); err != nil {
			return fmt.Errorf("failed to write the bootsector: %s", err)
		}
		fmt.Printf("> Bootsector { size: %d }\n", bootsectorSize)
	}

	fmt.Println("Done")
	return nil
}

func main() {
	cmd := &cli.Command{
		Name:  "mkimg",
		Usage: "make a disk image",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:    "partition",
				Aliases: []string{"p"},
				Usage:   "partition",
			},
			&cli.StringFlag{
				Name:    "name",
				Aliases: []string{"o", "dest"},
				Usage:   "path or name of destination image",
				Value:   "mkimg.img",
			},
			&cli.UintFlag{
				Name:  "first-sector",
				Usage: "sector number of the first partition",
				Value: 2048,
			},
			&cli.BoolFlag{
				Name:    "protective-mbr",
				Aliases: []string{"pmbr"},
				Usage:   "whether to set a protective mbr or not",
				Value:   false,
			},
			&cli.StringFlag{
				Name:     "bootsector",
				Usage:    "path to the bootsector to use",
				Required: false,
			},
		},
		Action: createDisk,
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		panic(err)
	}
}
