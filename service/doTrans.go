package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github/sxz799/gemini2chatgpt/model"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func DoTrans(apiKey string, openaiBody model.ChatGPTRequestBody, c *gin.Context) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	gModel := client.GenerativeModel("gemini-pro")
	cs := gModel.StartChat()
	cs.History = []*genai.Content{}
	lastMsg := ""
	for i, msg := range openaiBody.Messages {
		// 忽略 chat-next-web的默认提示词 You are ChatGPT, a large language model trained by OpenAI.\nCarefully heed the user's instructions. \nRespond using Markdown.
		if msg.Role == "system" && strings.Contains(msg.Content,"You are ChatGPT") {
			continue
		}
		content := msg.Content
		role:=msg.Role
		// 将assistant角色替换为model
		if msg.Role == "assistant" {
			role = "model"
		}
		if msg.Role == "system" {
			role = "user"
		}
		// 最后一条消息不写入历史记录 而是用于下一次请求
		if i == len(openaiBody.Messages)-1 {
			lastMsg = content
			break
		}
		cs.History = append(cs.History, &genai.Content{Parts: []genai.Part{genai.Text(content)}, Role: role})
	}

	if openaiBody.Stream {
		//支持 SSE特性
		c.Writer.Header().Set("Transfer-Encoding", "chunked")
		c.Writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		sendStreamResponse(cs, ctx, lastMsg, c)
	} else {
		sendSingleResponse(cs, ctx, lastMsg, c)
	}
}

func sendStreamResponse(cs *genai.ChatSession, ctx context.Context, lastMsg string, c *gin.Context) {
	iter := cs.SendMessageStream(ctx, genai.Text(lastMsg))
	for {
		resp, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			_, _ = c.Writer.WriteString("data: [DONE]\n")
			c.Writer.Flush()
			break
		}
		if err != nil {
			c.JSON(200,gin.H{
				"lastMsg":lastMsg,
				"err":err.Error(),
			})
			break
		}
		id := fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
		for _, candidate := range resp.Candidates {
			for _, p := range candidate.Content.Parts {
				str := fmt.Sprintf("%s", p)
				chunk := model.NewChatCompletionChunk(id, str, "gemini-pro")
				marshal, _ := json.Marshal(chunk)
				_, err = c.Writer.WriteString("data: " + string(marshal) + "\n\n")
				if err != nil {
					return
				}
				c.Writer.Flush()
			}
		}
	}
}

func sendSingleResponse(cs *genai.ChatSession, ctx context.Context, lastMsg string, c *gin.Context) {
	resp, err := cs.SendMessage(ctx, genai.Text(lastMsg))
	if err != nil {
		c.String(200, "SendMessage Error:", err.Error())
		return
	}
	if len(resp.Candidates) < 1 || len(resp.Candidates[0].Content.Parts) < 1 {
		c.String(200, "no response")
		return
	}
	part := resp.Candidates[0].Content.Parts[0]
	str := fmt.Sprintf("%s", part)
	cc := model.NewChatCompletion(str, "gemini-pro")
	marshal, _ := json.Marshal(cc)
	_, err = c.Writer.Write(marshal)
	if err != nil {
		return
	}
	c.Writer.Flush()
}
