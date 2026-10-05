Exercise 2: Shred tool in Go

For this exercise, it is necessary to install Golang – “sudo apt install golang-go”.

I was not able to implement all tests due to the given time frame limitation.

I implemented these 12 tests:

1. TestShredEmptyPath: if "" is passed to shred, it exits returning err != nil.
2. TestShredNonexistentFile: if a nonexistent path is passed to shred, it returns an error matching os.ErrNotExist.
3. TestShredDirectory: if a path to a folder is passed to shred, it returns fs.ErrInvalid and leaves the folder untouched.
4. TestShredEmptyFile: if the path to an empty file is passed to shred, it successfully removes the file.
5. TestShredNormalFile: if the path to a normal, non-empty, regular file is passed to shred, it succeeds.
6. TestShredOverwritesContent: verify that the contents of a normal file are indeed overwritten.
7. TestShredRefusesHardLinks: if a path to file that has more than one hard link is passed to shred, shred returns ErrMultipleNamesNotAllowed and leaves the file untouched.
8. TestShredOverwritesLargeFile: verifies that content overwrite works for a large file that would require multiple copy operations.
9. TestShredRefusesSymlink: if the path to a symbolic link is passed to shred, it returns an error and does nothing to both the link and its target.
10. TestShredPathChanged: verifies that ErrPathChanged is returned if the path changes between Lstat and OpenFile.
11. TestShredOverwriteKeepsSize: verifies that overwrite does not change the file size.
12. TestShredSpecialFile: verifies, using a FIFO, that non-regular files are rejected.

I could not implement these 6 tests:

1. TestShredOverwriteZeroLength: directly tests overwrite with a zero-byte file.
2. TestShredRelativePath: verifies that a valid relative path works correctly.
3. TestShredFileWithSpacesOrUnicode: verifies that unusual but valid filenames work.
4. TestShredRemoveFailure: verifies the error path if overwriting succeeds but os.Remove fails.
5. TestShredOverwriteError: verifies that an error during Seek, CopyN, or Sync is propagated.
6. TestShredPermissionDenied: verifies that shred returns an error when the file cannot be opened for writing.

Use case for shred:

One use case for shred is to delete files that contain sensitive information like bank accounts and passwords, for example.
When someone deletes a file from a computer storage, the file data may remain on the storage medium until the corresponding blocks are reused.
The filesystem marks the storage blocks previously associated with the file as available for reuse. While that doesn’t happen, all the content
of the deleted file could be recovered by someone with the specific knowledge. Therefore, it is necessary to overwrite all storage positions
that held the file before we can consider that the file has become really unavailable.

But why is it necessary to do it 3 times? Well, that is inherited from the time of Hard Disk Drives (HDDs), that worked magnetically. 
verwriting a bit on an HDD still leaves a faint “ghost magnetic imprint” of the previous bit, so it is still possible for someone
to recover the file. Each time we overwrite the file, the ghost magnetization of the original file becomes weaker. With 3 overwrites
it becomes so weak that it becomes impossible to recover the original content. 

For solid state storage, overwriting the file once should be enough, it it weren’t for a difficulty created by wear levelling and
internal block remapping: guaranteeing that the original physical flash cells have been overwritten is uncertain, if not impossible.
