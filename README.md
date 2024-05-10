# godrop

I was annoyed by the lack of a simple service that I could run on my own server, to let other people upload files to me. So I made one in one day.

Nextcloud file drop is great but I somewhat dislike Nextcloud and wanted something simpler.

## TODO

### server

- add environment variables for configuration
- make sure uploading a file with the same name at the same time from different clients doesn't overwrite the file
- clean up cancelled uploads (error reading chunk: unexpected EOF)

### ui

- handle (ignore?) dropped folders

### other

- add a license
- add a readme
