package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var (
	apiKey     string
	apiURL     string
	model      string
	galleryDir string
)

type imageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n"`
	Size           string `json:"size,omitempty"`
	ResponseFormat string `json:"response_format"`
}

type imageDataItem struct {
	B64JSON string `json:"b64_json,omitempty"`
	URL     string `json:"url,omitempty"`
}

type imageResponse struct {
	Data  []imageDataItem `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func generateImage(prompt, size string) (b64 string, mimeType string, filename string, err error) {
	reqBody := imageRequest{
		Model:          model,
		Prompt:         prompt,
		N:              1,
		Size:           size,
		ResponseFormat: "b64_json",
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", "", "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var imgResp imageResponse
	if err := json.Unmarshal(respBytes, &imgResp); err != nil {
		return "", "", "", fmt.Errorf("unmarshal: %w (body: %.200s)", err, string(respBytes))
	}

	if imgResp.Error != nil {
		return "", "", "", fmt.Errorf("API: %s", imgResp.Error.Message)
	}

	if len(imgResp.Data) == 0 {
		return "", "", "", fmt.Errorf("no image data in response")
	}

	item := imgResp.Data[0]
	var imgBytes []byte

	if item.B64JSON != "" {
		b64 = item.B64JSON
		imgBytes, err = base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return "", "", "", fmt.Errorf("decode base64: %w", err)
		}
	} else if item.URL != "" {
		dlResp, dlErr := http.Get(item.URL)
		if dlErr != nil {
			return "", "", "", fmt.Errorf("download: %w", dlErr)
		}
		defer dlResp.Body.Close()
		imgBytes, err = io.ReadAll(dlResp.Body)
		if err != nil {
			return "", "", "", fmt.Errorf("read download: %w", err)
		}
		b64 = base64.StdEncoding.EncodeToString(imgBytes)
	} else {
		return "", "", "", fmt.Errorf("response contains neither b64_json nor url")
	}

	// Detect MIME type
	mimeType = "image/png"
	if len(imgBytes) > 2 && imgBytes[0] == 0xFF && imgBytes[1] == 0xD8 {
		mimeType = "image/jpeg"
	}

	ext := ".png"
	if mimeType == "image/jpeg" {
		ext = ".jpg"
	}

	// Save to gallery
	filename = time.Now().Format("2006-01-02-150405") + ext
	savePath := filepath.Join(galleryDir, filename)

	if mkErr := os.MkdirAll(galleryDir, 0755); mkErr != nil {
		log.Printf("Warning: mkdir gallery: %v", mkErr)
	} else if wErr := os.WriteFile(savePath, imgBytes, 0644); wErr != nil {
		log.Printf("Warning: save image: %v", wErr)
	} else {
		log.Printf("Saved: %s (%d bytes)", savePath, len(imgBytes))
	}

	return b64, mimeType, filename, nil
}

func main() {
	apiKey = os.Getenv("AISTUDIO_API_KEY")
	if apiKey == "" {
		log.Fatal("AISTUDIO_API_KEY is required")
	}

	apiURL = os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "https://aistudio.baidu.com/llm/lmapi/v3/images/generations"
	}

	model = os.Getenv("MODEL")
	if model == "" {
		model = "ERNIE-Image-Turbo"
	}

	galleryDir = os.Getenv("GALLERY_DIR")
	if galleryDir == "" {
		galleryDir = "/home/ubuntu/imagegen-mcp/gallery"
	}

	port := "8088"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	s := server.NewMCPServer(
		"ImageGen MCP",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithLogging(),
	)

	genTool := mcp.NewTool("generate_image",
		mcp.WithDescription("根据文字描述生成图片。支持中英文prompt，描述越详细效果越好。图片会保存到画廊并返回预览。"),
		mcp.WithString("prompt",
			mcp.Required(),
			mcp.Description("图片描述，支持中英文"),
		),
		mcp.WithString("size",
			mcp.Description("图片尺寸，默认1024x1024。可选：512x512、768x768、1024x1024"),
		),
	)

	s.AddTool(genTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		prompt, _ := args["prompt"].(string)
		if prompt == "" {
			return mcp.NewToolResultError("prompt is required"), nil
		}

		size, _ := args["size"].(string)
		if size == "" {
			size = "1024x1024"
		}

		log.Printf("Generating: prompt=%q size=%s", prompt, size)

		b64Data, mimeType, filename, err := generateImage(prompt, size)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("生成失败: %v", err)), nil
		}

		galleryURL := fmt.Sprintf("https://qiyun.cloud/gallery/%s", filename)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.ImageContent{
					Type:     "image",
					Data:     b64Data,
					MIMEType: mimeType,
				},
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("画廊: %s", galleryURL),
				},
			},
		}, nil
	})

	httpServer := server.NewStreamableHTTPServer(s,
		server.WithStateLess(true),
	)

	addr := ":" + port
	log.Printf("ImageGen MCP listening on %s/mcp", addr)
	if err := httpServer.Start(addr); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
