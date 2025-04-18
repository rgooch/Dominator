# imagetool
A utility to add, delete, get and compare images on the
*[imageserver](../imageserver/README.md)*.

The *imagetool* is the most important utility in the **Dominator** system, as it
is used to manage images. *Imagetool* may be run on any machine. It is typically
run on a desktop, bastion or build machine, depending on the sophistication of
your build environment.

## Usage
*Imagetool* supports several sub-commands. There are many command-line flags
which provide parameters for these sub-commands. The most commonly used
parameter is `-imageServerHostname` which specifies which host the *imageserver*
to talk to is running on. The basic usage pattern is:

```
imagetool [flags...] command [args...]
```

Built-in help is available with the command:

```
imagetool -h
```

Some of the sub-commands available are:

- **add**: add an image using a compressed tarfile for image data
- **addi**: add an image using an existing image for image data
- **addrep**: add an image using an existing image and layer files from
              compressed tarfiles on top of existing files
- **adds**: add an image using files from a running *subd* for image data (this
            allows "snapshotting" of a golden machine)
- **bulk-addrep**: perform addrep operation for all images
- **change-image-expiration**: change or remove the expiration time for an image
- **check**: check if an image exists
- **check-directory**: check if a directory exists
- **chown**: change the owner group of an image directory
- **copy**: copy an image
- **copy-filtered-files**: copy files from a directory tree which match the image filter
- **delete**: delete an image
- **delunrefobj**: delete (garbage collect) unreferenced objects
- **diff**: compare two images
- **diff-build-logs**: compare the build logs for two images
- **diff-files**: compare the specified file in two images
- **diff-filters**: compare the filters for two images
- **diff-package-lists**: compare the package lists for two images
- **diff-triggers**: compare the triggers for two images
- **estimate-usage**: estimate the file-system space needed to unpack an image
- **find-latest-image**: find the latest image in a directory
- **get**: get and unpack an image
- **get-archive-data**: get archive (audit) data for an image
- **get-build-log**: get build log for an image
- **get-file-in-image**: get file in an image
- **get-image-expiration**: get the expiration time for an image
- **get-image-updates**: get a stream of image updates
- **get-package-list**: get package list for an image
- **get-replication-master**: show the replication master for the imageserver
- **list**: list all images
- **list-mdb**: list all image names in the MDB (images may not exist)
- **list-not-in-mdb**: list all images not listed in the MDB
- **listdirs**: list all directories
- **listunrefobj**: list the unreferenced objects on the server
- **make-raw-image**: make a bootable RAW image from an image
- **match-triggers**: match a path to a triggers file
- **merge-filters**: merge filter files
- **merge-triggers**: merge trigger files
- **mkdir**: make a directory
- **patch-directory**: patch (update) a local directory with an image
- **restore-from-file**: restore an image from an imagearchive file
- **save-to-file**: save an image to an imagearchive file or stdout
- **scan-filtered-files**: scan a directory and list those matched by the image filter
- **show**: show (list) an image
- **show-bad-computed-files**: show the subs (and their images) which want
                               computed files which are not available
- **show-bad-image-subs**: show the subs which have missing or expired images
- **show-computed-file-subs**: show the subs (and their images) which should
                               receive the specified computed file. This is
			       useful if you want to deprecate a computed file
			       and need to see where it is being used
- **show-filter**: show the filter for an image
- **show-inode**: show metadata for an inode in an image
- **show-metadata**: show metadata for an image
- **show-triggers**: show triggers for an image
- **showunrefobj**: list the unreferenced objects on the server and their sizes
- **tar**: create a tarfile from an image
- **test-download-speed**: test the speed for downloading objects for an image
- **trace-inode-history**: trace the change history of an inode in an image and its sources
- **wait**: wait (with timeout) for an image to exist

## Security
*[Imageserver](../imageserver/README.md)* restricts RPC access using TLS client
authentication. *Imagetool* will load certificate and key files from the
`~/.ssl` directory. *Imagetool* will present these certificates to
*imageserver*. If one of the certificates is signed by a certificate authority
that *imageserver* trusts, *imageserver* will grant access.
