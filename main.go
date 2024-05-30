package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var uploadPath = "./uploads"
var tempPath = filepath.Join(uploadPath, ".temp")

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

		// Prepare the filename for the final location (duplicate check)
		uploadFilePath := filepath.Join(uploadPath, fileName)
		if _, err := os.Stat(uploadFilePath); err == nil {
			log.Println("File exists", uploadFilePath)
			// File already exists, append a number to the filename
			fileExt := filepath.Ext(fileName)
			fileNameWithoutExt := fileName[:len(fileName)-len(fileExt)]
			uploadFilePath = filepath.Join("./uploads", fileNameWithoutExt+"-1"+fileExt)
			i := 2
			for {
				if _, err := os.Stat(uploadFilePath); os.IsNotExist(err) {
					break
				}
				uploadFilePath = filepath.Join("./uploads", fileNameWithoutExt+"-"+strconv.Itoa(i)+fileExt)
				i++
			}
		}

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

func main() {
	err := os.MkdirAll(uploadPath, os.ModePerm)
	if err != nil {
		log.Printf("Error creating uploads folder: %s", err.Error())
	}

	err = os.RemoveAll(tempPath)
	if err != nil {
		log.Printf("Error clearing temp uploads folder: %s", err.Error())
	}
	err = os.MkdirAll(tempPath, os.ModePerm)
	if err != nil {
		log.Printf("Error creating temp uploads folder: %s", err.Error())
	}

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/upload", uploadHandler)
	fmt.Println("Starting server at http://localhost:7598")
	err = http.ListenAndServe(":7598", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}
