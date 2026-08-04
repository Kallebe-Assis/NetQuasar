package backupb2

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Creds — credenciais B2.
type Creds struct {
	KeyID          string
	ApplicationKey string
	Bucket         string
	BucketID       string
	Endpoint       string
	Region         string
	Prefix         string
}

type authResp struct {
	AccountID          string `json:"accountId"`
	AuthorizationToken string `json:"authorizationToken"`
	APIURL             string `json:"apiUrl"`
	DownloadURL        string `json:"downloadUrl"`
	Allowed            struct {
		BucketID string `json:"bucketId"`
	} `json:"allowed"`
}

// Client API nativa B2.
type Client struct {
	http  *http.Client
	auth  authResp
	creds Creds
}

// NewClient autentica na conta B2.
func NewClient(ctx context.Context, c Creds) (*Client, error) {
	c.KeyID = strings.TrimSpace(c.KeyID)
	c.ApplicationKey = strings.TrimSpace(c.ApplicationKey)
	c.Bucket = strings.TrimSpace(c.Bucket)
	c.Prefix = strings.Trim(strings.TrimSpace(c.Prefix), "/")
	if c.Prefix == "" {
		c.Prefix = "netquasar/postgres"
	}
	if c.KeyID == "" || c.ApplicationKey == "" || c.Bucket == "" {
		return nil, fmt.Errorf("B2: key_id, application_key e bucket são obrigatórios")
	}
	pair := c.KeyID + ":" + c.ApplicationKey
	b64 := base64.StdEncoding.EncodeToString([]byte(pair))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.backblazeb2.com/b2api/v2/b2_authorize_account", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Basic "+b64)
	cli := &http.Client{Timeout: 60 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("b2_authorize_account: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var ar authResp
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, err
	}
	cl := &Client{http: cli, auth: ar, creds: c}
	if err := cl.resolveBucketID(ctx); err != nil {
		return nil, err
	}
	return cl, nil
}

func (c *Client) resolveBucketID(ctx context.Context) error {
	if id := strings.TrimSpace(c.creds.BucketID); id != "" {
		c.auth.Allowed.BucketID = id
		return nil
	}
	if c.auth.Allowed.BucketID != "" {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"accountId":  c.auth.AccountID,
		"bucketName": c.creds.Bucket,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.auth.APIURL+"/b2api/v2/b2_list_buckets", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.auth.AuthorizationToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("b2_list_buckets: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var lr struct {
		Buckets []struct {
			BucketID   string `json:"bucketId"`
			BucketName string `json:"bucketName"`
		} `json:"buckets"`
	}
	if err := json.Unmarshal(body, &lr); err != nil {
		return err
	}
	for _, b := range lr.Buckets {
		if b.BucketName == c.creds.Bucket {
			c.auth.Allowed.BucketID = b.BucketID
			c.creds.BucketID = b.BucketID
			return nil
		}
	}
	return fmt.Errorf("bucket B2 %q não encontrado", c.creds.Bucket)
}

// FileInfo — objecto no B2.
type FileInfo struct {
	FileID          string `json:"file_id"`
	FileName        string `json:"file_name"`
	ContentLength   int64  `json:"content_length"`
	UploadTimestamp int64  `json:"upload_timestamp"`
	BaseName        string `json:"base_name"`
}

// ListDumps lista netquasar-full-*.pgdump no prefixo.
func (c *Client) ListDumps(ctx context.Context) ([]FileInfo, error) {
	prefix := c.creds.Prefix + "/"
	var out []FileInfo
	var start *string
	for {
		bodyMap := map[string]any{
			"bucketId":     c.auth.Allowed.BucketID,
			"prefix":       prefix,
			"maxFileCount": 1000,
		}
		if start != nil {
			bodyMap["startFileName"] = *start
		}
		payload, _ := json.Marshal(bodyMap)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.auth.APIURL+"/b2api/v2/b2_list_file_names", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", c.auth.AuthorizationToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("b2_list_file_names: HTTP %d: %s", resp.StatusCode, string(raw))
		}
		var lr struct {
			Files []struct {
				FileID          string `json:"fileId"`
				FileName        string `json:"fileName"`
				ContentLength   int64  `json:"contentLength"`
				UploadTimestamp int64  `json:"uploadTimestamp"`
				Action          string `json:"action"`
			} `json:"files"`
			NextFileName *string `json:"nextFileName"`
		}
		if err := json.Unmarshal(raw, &lr); err != nil {
			return nil, err
		}
		for _, f := range lr.Files {
			if f.Action != "upload" {
				continue
			}
			bn := filepath.Base(f.FileName)
			if !strings.HasPrefix(bn, "netquasar-full-") || !strings.HasSuffix(bn, ".pgdump") {
				continue
			}
			out = append(out, FileInfo{
				FileID: f.FileID, FileName: f.FileName, ContentLength: f.ContentLength,
				UploadTimestamp: f.UploadTimestamp, BaseName: bn,
			})
		}
		if lr.NextFileName == nil || *lr.NextFileName == "" {
			break
		}
		start = lr.NextFileName
	}
	return out, nil
}

// UploadFile envia ficheiro para o prefixo B2.
func (c *Client) UploadFile(ctx context.Context, localPath, remoteName string) (fileID string, size int64, err error) {
	remoteName = strings.TrimPrefix(remoteName, "/")
	if !strings.Contains(remoteName, "/") {
		remoteName = c.creds.Prefix + "/" + remoteName
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", 0, err
	}
	sum := sha1.Sum(data)
	sha1hex := hex.EncodeToString(sum[:])

	payload, _ := json.Marshal(map[string]any{"bucketId": c.auth.Allowed.BucketID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.auth.APIURL+"/b2api/v2/b2_get_upload_url", bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", c.auth.AuthorizationToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, err
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("b2_get_upload_url: HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var ur struct {
		UploadURL          string `json:"uploadUrl"`
		AuthorizationToken string `json:"authorizationToken"`
	}
	if err := json.Unmarshal(raw, &ur); err != nil {
		return "", 0, err
	}

	upReq, err := http.NewRequestWithContext(ctx, http.MethodPost, ur.UploadURL, bytes.NewReader(data))
	if err != nil {
		return "", 0, err
	}
	upReq.Header.Set("Authorization", ur.AuthorizationToken)
	parts := strings.Split(remoteName, "/")
	for i, p := range parts {
		parts[i] = url.QueryEscape(p)
		parts[i] = strings.ReplaceAll(parts[i], "+", "%20")
	}
	upReq.Header.Set("X-Bz-File-Name", strings.Join(parts, "/"))
	upReq.Header.Set("Content-Type", "application/octet-stream")
	upReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
	upReq.Header.Set("X-Bz-Content-Sha1", sha1hex)
	upReq.ContentLength = int64(len(data))

	upCli := &http.Client{Timeout: 30 * time.Minute}
	upResp, err := upCli.Do(upReq)
	if err != nil {
		return "", 0, err
	}
	defer upResp.Body.Close()
	upBody, _ := io.ReadAll(upResp.Body)
	if upResp.StatusCode != 200 {
		return "", 0, fmt.Errorf("b2 upload: HTTP %d: %s", upResp.StatusCode, string(upBody))
	}
	var ur2 struct {
		FileID string `json:"fileId"`
	}
	_ = json.Unmarshal(upBody, &ur2)
	return ur2.FileID, int64(len(data)), nil
}

// DownloadToFile descarrega objecto B2 para disco.
func (c *Client) DownloadToFile(ctx context.Context, remoteName, localPath string) error {
	remoteName = strings.TrimPrefix(remoteName, "/")
	parts := strings.Split(remoteName, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	u := strings.TrimRight(c.auth.DownloadURL, "/") + "/file/" + url.PathEscape(c.creds.Bucket) + "/" + strings.Join(parts, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.auth.AuthorizationToken)
	cli := &http.Client{Timeout: 30 * time.Minute}
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("b2 download: HTTP %d: %s", resp.StatusCode, string(b))
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// ObjectKeyForDump monta o caminho remoto.
func ObjectKeyForDump(prefix, fileName string) string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		prefix = "netquasar/postgres"
	}
	return prefix + "/" + filepath.Base(fileName)
}

// StampFileName nome padrão do dump.
func StampFileName(t time.Time) string {
	return fmt.Sprintf("netquasar-full-%s.pgdump", t.Format("20060102-1504"))
}
