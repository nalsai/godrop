package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/tus/tusd/v2/pkg/filelocker"
	"github.com/tus/tusd/v2/pkg/filestore"
	"github.com/tus/tusd/v2/pkg/handler"
)

var uploadPath = "./uploads"
var tusPath = filepath.Join(uploadPath, "tus")
var tempPath = filepath.Join(uploadPath, "tmp")

func sanitizeName(s string) string {
	s = strings.Replace(s, "/", "_", -1)

	if strings.HasPrefix(s, ".") {
		s = "_" + strings.TrimPrefix(s, ".")
	}
	if strings.HasSuffix(s, ".") {
		s = strings.TrimSuffix(s, ".") + "_"
	}
	return s
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("File Upload Endpoint Hit")

	multipartReader, err := r.MultipartReader()
	if err != nil {
		log.Printf("failed to get a multipart reader %v", err)
		http.Error(w, "Error: failed to get a multipart reader", http.StatusInternalServerError)
		return
	}

	for {
		var uploaded bool
		var filesize int
		var bytesHandled int
		buffer := make([]byte, 4096)

		err := checkDiskSpace(uploadPath)
		if err != nil {
			log.Printf("Error: %s", err.Error())
			http.Error(w, "Error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		part, err := multipartReader.NextPart()
		if err != nil {
			if err != io.EOF {
				log.Printf("error fetching next part %v", err)
				http.Error(w, "Error fetching next part", http.StatusInternalServerError)
				return
			}
			break // just an eof, not an error
		}

		fileName := sanitizeName(part.FileName())
		log.Printf("Handling %s", fileName)

		os.MkdirAll(tempPath, os.ModePerm)
		filePath := filepath.Join(tempPath, fileName)
		fileDrain, err := os.Create(filePath)
		if err != nil {
			log.Printf("error creating file %s, %v", fileName, err)
			http.Error(w, "Error creating file", http.StatusInternalServerError)
			return
		}
		defer fileDrain.Close()

		for !uploaded {
			if bytesHandled, err = part.Read(buffer); err != nil {
				if err != io.EOF {
					log.Printf("error reading chunk: %s", err.Error())
					http.Error(w, "Error reading chunk", http.StatusInternalServerError)
					return
				}
				uploaded = true
			}

			if bytesHandled, err = fileDrain.Write(buffer[:bytesHandled]); err != nil {
				log.Printf("error writing chunk: %s", err.Error())
				http.Error(w, "Error writing chunk", http.StatusInternalServerError)
				return
			}
			filesize += bytesHandled
		}
		log.Printf("Uploaded %d bytes for %s", filesize, fileName)

		// Ensure unique name (avoid overwrite)
		uploadFilePath := uniqueFilename(filepath.Join(uploadPath, fileName))

		// Move the file to the final location
		os.MkdirAll(uploadPath, os.ModePerm)
		if err = os.Rename(filePath, uploadFilePath); err != nil {
			log.Printf("cannot move file %s to uploads %v", fileName, err)
			http.Error(w, "Error: cannot move file from temp to uploads", http.StatusInternalServerError)
		}
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Upload completed")
}

func uniqueFilename(path string) string {
	ext := filepath.Ext(path)
	name := strings.TrimSuffix(filepath.Base(path), ext)
	dir := filepath.Dir(path)

	newPath := path
	i := 1
	for {
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			break
		}
		newPath = filepath.Join(dir, fmt.Sprintf("%s_%d%s", name, i, ext))
		i++
	}
	return newPath
}

func main() {
	err := os.MkdirAll(uploadPath, os.ModePerm)
	if err != nil {
		log.Printf("Error creating uploads folder: %s", err.Error())
	}
	err = os.MkdirAll(tusPath, os.ModePerm)
	if err != nil {
		log.Printf("Error creating tus folder: %s", err.Error())
	}

	err = os.RemoveAll(tempPath)
	if err != nil {
		log.Printf("Error clearing temp uploads folder: %s", err.Error())
	}
	err = os.MkdirAll(tempPath, os.ModePerm)
	if err != nil {
		log.Printf("Error creating temp uploads folder: %s", err.Error())
	}

	// Set up tusd data store
	store := filestore.New(tusPath)
	locker := filelocker.New(tusPath)
	composer := handler.NewStoreComposer()
	store.UseIn(composer)
	locker.UseIn(composer)

	handler, err := handler.NewHandler(handler.Config{
		BasePath:              "/uploadtus/",
		StoreComposer:         composer,
		NotifyCompleteUploads: true,
	})
	if err != nil {
		log.Fatalf("unable to create handler: %s", err)
	}

	go func() {
		for {
			event := <-handler.CompleteUploads

			// Get original filename from metadata (sent by client via Upload-Metadata)
			origName := event.Upload.MetaData["filename"]
			if origName == "" {
				origName = event.Upload.ID // fallback
			}

			origName = sanitizeName(origName)

			// Ensure unique name (avoid overwrite)
			finalName := uniqueFilename(filepath.Join(uploadPath, origName))

			// Move file from tus temp folder to final folder
			src := filepath.Join(tusPath, event.Upload.ID)
			if err := os.Rename(src, finalName); err != nil {
				log.Println("Error moving file:", err)
				continue
			}

			// Remove to info file
			infoFile := filepath.Join(tusPath, event.Upload.ID+".info")
			if err := os.Remove(infoFile); err != nil {
				log.Printf("Error removing info file %s: %s", infoFile, err.Error())
				continue
			}

			log.Printf("TUS upload %s saved as %s", event.Upload.ID, finalName)
		}
	}()

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/upload", uploadHandler)
	http.Handle("/uploadtus/", http.StripPrefix("/uploadtus/", handler))
	http.Handle("/uploadtus", http.StripPrefix("/uploadtus", handler))

	fmt.Println("Starting server at http://localhost:7598")
	err = http.ListenAndServe(":7598", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

// Function to check disk space and throw error if below 5%
func checkDiskSpace(path string) error {
	var stat syscall.Statfs_t

	// Get filesystem stats
	if err := syscall.Statfs(path, &stat); err != nil {
		return fmt.Errorf("failed to get filesystem stats: %v", err)
	}

	// Calculate total and free blocks
	totalBlocks := stat.Blocks
	freeBlocks := stat.Bfree

	// Calculate the percentage of free space
	freePercent := (float64(freeBlocks) / float64(totalBlocks)) * 100

	// Check if free space is below 5%
	if freePercent < 5.0 {
		return fmt.Errorf("free disk space below 5%%: currently at %.2f%%", freePercent)
	}

	// Free space is adequate
	return nil
}
