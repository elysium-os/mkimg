# mkimg
mkimg is a tiny tool to simplify the process of creating partitioned disk images. The general idea is to setup a config describing the image you want to create and let mkimg do the work for you.

## Usage
`mkimg [options]`

## Options
`--config=<file>` overrides the default config file path of _./mkimg.toml_  

## Configuration
The configuration consists of one toml file which contains information about the image and its partitions. Each `[[partitions]]` entry describes one partition (they are interpreted in order). Below is a comprehensive list of the configuration options provided by mkimg.
```toml
# (Optional) The name of the image output by mkimg
name = "awesome.img"

# (Optional) Use a protective mbr?
protective-mbr = true

# (Optional) Path to a bootsector (max 440 byte binary) to be written to the image
bootsector = "./bootsector.bin"


# Common partition properties
[[partitions]]
# (Optional) Partition Name
name = "root"

# Partition GPT type
gpt-type = "01233445-1234-5432-1234-ABCDEF123456"

# Partition mkimg type
# Types: "fs", "file"
type = "fs"
...


# "fs" partition properties
[[partitions]]
# Filesystem type
# Types: "fat32"
fs-type = "fat32"

# Filesystem size in mb
size = 128

# Files to copy onto filesystem
# If the file is a directory, its contents are copied into the root. If it is a file, the file is copied into the root
files = ["root", "boot.cfg"]
...


# "file" partition properties
[[partitions]]
# Path to the file which will be written into the partition
file = "./helloworld.bin"
...
```