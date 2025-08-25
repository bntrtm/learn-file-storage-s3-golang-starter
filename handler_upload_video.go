package main

import (
	"io"
	"os"
	"log"
	"fmt"
	"mime"
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}
	
	fmt.Println("uploading video:", videoID, "by user", userID)

	vMetadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}
	if vMetadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}
	const maxMemory = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxMemory)
	err = r.ParseMultipartForm(10 << 20)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}
	defer file.Close()

	extension, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if extension != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}

	//assetPath := getAssetPath(videoIDString, extension)
	//assetDiskPath := cfg.getAssetDiskPath(assetPath)

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}
	defer os.Remove("tubely-upload.mp4")
	defer tempFile.Close() // defer is LIFO

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}

	// reset temp file's file pointer to beginning so it can be read again
	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		log.Print("Failure to reset tempFile pointer to beginning")
	}
	
	randFilename, err := makeFilename()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}

	putObjectParams := s3.PutObjectInput{
		Bucket:			&(cfg.s3Bucket),
		Key:			&(randFilename),
		Body:			tempFile,
		ContentType:	&extension,
	}
	_, err = cfg.s3Client.PutObject(context.Background(), &putObjectParams)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}

	// save path on disk into video metadata
	objectUrl := cfg.getS3URL(randFilename)
	log.Printf("Saving new video at: %s", objectUrl)
	vMetadata.VideoURL = &objectUrl
	
	err = cfg.db.UpdateVideo(vMetadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong", err)
		return
	}

	respondWithJSON(w, http.StatusOK, vMetadata)
	return
}