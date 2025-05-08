package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	http.MaxBytesReader(w, r.Body, 1 << 30)

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

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

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", nil)
		return
	}

	multipartFile, _, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error getting file from form", nil)
		return
	}

	defer multipartFile.Close()
	
	mimeType, _, err := mime.ParseMediaType("video/mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error parsing mime type", nil)
		return
	}

	if mimeType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Only accepts video/mp4 type", nil)
		return
	}

	file, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error creating temp file", nil)
		return
	}

	defer os.Remove(file.Name())
	defer file.Close()

	io.Copy(file, multipartFile)

	file.Seek(0, io.SeekStart)

	aspectRatio, err := getVideoAspectRatio(file.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error getting aspect ratio", nil)
		return
	}

	key := make([]byte, 32)
	rand.Read(key)
	objKey :=  "/" + aspectRatio + "/" + base64.RawURLEncoding.EncodeToString(key)

	cfg.s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: &cfg.s3Bucket,
		Key: &objKey,
		Body: file,
		ContentType: &mimeType,
	})

	url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, objKey)
	video.VideoURL = &url

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
