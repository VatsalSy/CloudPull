# Google Drive API Research Notes

## API Quotas and Limits

### Per-User Limits

- **Queries per 100 seconds**: 1,000
- **Queries per 100 seconds per user**: 1,000
- **Upload bandwidth per user**: 750 GB/day
- **Download bandwidth**: Unlimited (but subject to rate limiting)

### Best Practices for Quota Management

1. Use exponential backoff (2^n * 1000ms + random_ms)
2. Batch requests when possible (max 100 per batch)
3. Use fields parameter to limit response size
4. Cache metadata aggressively

## Key API Endpoints

### 1. Files List

```http
GET https://www.googleapis.com/drive/v3/files
Parameters:
- q: Query string (e.g., "'folder-id' in parents")
- pageSize: Max 1000
- pageToken: For pagination
- fields: Specify fields to return
- orderBy: Sort results
```

### 2. Files Get (Metadata)

```http
GET https://www.googleapis.com/drive/v3/files/{fileId}
Parameters:
- fields: Specify fields (id,name,size,md5Checksum,mimeType,modifiedTime)
```

### 3. Files Download

```http
GET https://www.googleapis.com/drive/v3/files/{fileId}?alt=media
Headers:
- Range: bytes=start-end (for resumable downloads)
```

### 4. Files Export (Google Docs)

```http
GET https://www.googleapis.com/drive/v3/files/{fileId}/export
Parameters:
- mimeType: Target format
```

## Special Considerations

### Google Workspace Files

These files don't have binary content and must be exported:

| Source Type | Recommended Export | Size Limit |
|------------|-------------------|------------|
| Google Docs | application/pdf or .docx | 10 MB |
| Google Sheets | application/vnd.openxmlformats-officedocument.spreadsheetml.sheet | 100 MB |
| Google Slides | application/pdf or .pptx | 100 MB |
| Google Drawings | application/pdf or .png | 10 MB |

### File Types and MIME Types

```go
var GoogleMimeTypes = map[string]string{
    "application/vnd.google-apps.document":     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    "application/vnd.google-apps.spreadsheet":  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    "application/vnd.google-apps.presentation": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
    "application/vnd.google-apps.drawing":      "application/pdf",
}
```

### Handling Large Downloads

1. Use `Range` header for chunked downloads
2. Store download progress in database
3. Implement retry logic for failed chunks
4. Verify with MD5 checksum after completion

### Rate Limit Response

```json
{
  "error": {
    "errors": [
      {
        "domain": "usageLimits",
        "reason": "userRateLimitExceeded",
        "message": "User Rate Limit Exceeded"
      }
    ],
    "code": 403,
    "message": "User Rate Limit Exceeded"
  }
}
```

### Optimization Techniques

1. **Batch Requests**

```go
batch := drive.NewBatchRequest()
for _, fileId := range fileIds {
    req := service.Files.Get(fileId).Fields("id,name,size,md5Checksum")
    batch.Add(req, callback)
}
batch.Do()
```

1. **Partial Response**

Always use `fields` parameter:

```http
fields=files(id,name,size,md5Checksum,mimeType,parents,modifiedTime)
```

1. **Change Detection**

Use `modifiedTime` and `md5Checksum` to skip unchanged files

## Authentication Flow

### OAuth2 Scopes Required

- `https://www.googleapis.com/auth/drive.readonly` - Read-only access

### Token Storage

- Store refresh token securely
- Implement automatic token refresh
- Handle token expiration gracefully
