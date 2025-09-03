# mkimg

mkimg is a tiny tool to simplify the process of creating partitioned disk images.

## Usage

mkimg [options]

`--partition string`, `-p string`  
Create a partition in the image (see [more](#partition)).

`--name string`, `--dest string`, `-o string`  
Path or name of destination image (default: "mkimg.img").

`--first-sector uint`  
Sector number of the first partition (default: 2048).

`--protective-mbr`, `--pmbr`  
Whether to set a protective mbr or not (default: false).

`--bootsector string`  
Path to a bootsector if wanted.

`--help`, `-h`  
Show help.

## Partition

The partition option takes a set of key value entries separated by colons. The key values are separated by equal signs. The valid keys are:

| Key        | Value                    | Partition Type | Description                                                                                                                                                                                                                                                                                                                                          |
|------------|--------------------------|----------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `type`     | `fs`, `file`             |                | The type of the partition.                                                                                                                                                                                                                                                                                                                           |
| `name`     | string                   |                | The name of the partition.                                                                                                                                                                                                                                                                                                                           |
| `gpt-type` | string (UUID)            |                | GPT type of the partition.                                                                                                                                                                                                                                                                                                                           |
| `file`     | string (path)            | `file`         | Path to a file that will be written into the partition as raw data.                                                                                                                                                                                                                                                                                  |
| `fs-type`  | `fat32`                  | `fs`           | Filesystem type of the FS partition.                                                                                                                                                                                                                                                                                                                 |
| `fs-size`  | number (mb)              | `fs`           | Size of the FS partition in mb.                                                                                                                                                                                                                                                                                                                      |
| `fs-root`  | string (path)            | `fs`           | Path to a directory that will be copied in as the root of the FS partition.                                                                                                                                                                                                                                                                          |
| `fs-files` | string (see description) | `fs`           | List of files or directories that will be copied into the FS partition. Paths in the string are separated by a #. An explicit destination can be defined by adding an @ followed by a path after the source path. If no explicit destination has been defined the destination will be the basename of the source path at the root of the filesystem. |
