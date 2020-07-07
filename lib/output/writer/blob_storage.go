package writer

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Jeffail/benthos/v3/lib/bloblang/x/field"
	"github.com/Jeffail/benthos/v3/lib/log"
	"github.com/Jeffail/benthos/v3/lib/metrics"
	"github.com/Jeffail/benthos/v3/lib/types"
)

//------------------------------------------------------------------------------

// AzureBlobStorageConfig contains configuration fields for the AzureBlobStorage output type.
type AzureBlobStorageConfig struct {
	StorageAccount   string `json:"storage_account" yaml:"storage_account"`
	StorageAccessKey string `json:"storage_access_key" yaml:"storage_access_key"`
	Container        string `json:"container" yaml:"container"`
	Path             string `json:"path" yaml:"path"`
	BlobType         string `json:"blob_type" yaml:"blob_type"`
	Timeout          string `json:"timeout" yaml:"timeout"`
	MaxInFlight      int    `json:"max_in_flight" yaml:"max_in_flight"`
}

// NewAzureBlobStorageConfig creates a new Config with default values.
func NewAzureBlobStorageConfig() AzureBlobStorageConfig {
	return AzureBlobStorageConfig{
		StorageAccount:   "",
		StorageAccessKey: "",
		Container:        "",
		Path:             `${!count("files")}-${!timestamp_unix_nano()}.txt`,
		BlobType:         "BLOCK",
		Timeout:          "5s",
		MaxInFlight:      1,
	}
}

//------------------------------------------------------------------------------

// AzureBlobStorage is a benthos writer. Type implementation that writes messages to an
// Azure Blob Storage storage account.
type AzureBlobStorage struct {
	conf      AzureBlobStorageConfig
	container field.Expression
	path      field.Expression
	blobType  field.Expression
	timeout   time.Duration
	log       log.Modular
	stats     metrics.Type
}

// NewAzureBlobStorage creates a new Amazon S3 bucket writer.Type.
func NewAzureBlobStorage(
	conf AzureBlobStorageConfig,
	log log.Modular,
	stats metrics.Type,
) (*AzureBlobStorage, error) {
	var timeout time.Duration
	if tout := conf.Timeout; len(tout) > 0 {
		var err error
		if timeout, err = time.ParseDuration(tout); err != nil {
			return nil, fmt.Errorf("failed to parse timeout period string: %v", err)
		}
	}
	a := &AzureBlobStorage{
		conf:    conf,
		log:     log,
		stats:   stats,
		timeout: timeout,
	}
	var err error
	if a.container, err = field.New(conf.Container); err != nil {
		return nil, fmt.Errorf("failed to parse container expression: %v", err)
	}
	if a.path, err = field.New(conf.Path); err != nil {
		return nil, fmt.Errorf("failed to parse path expression: %v", err)
	}
	if a.blobType, err = field.New(conf.BlobType); err != nil {
		return nil, fmt.Errorf("failed to parse blob type expression: %v", err)
	}
	return a, nil
}

// ConnectWithContext attempts to establish a connection to the target Blob Storage Account.
func (a *AzureBlobStorage) ConnectWithContext(ctx context.Context) error {
	return a.Connect()
}

// Connect attempts to establish a connection to the target Blob Storage Account.
func (a *AzureBlobStorage) Connect() error {
	return nil
}

// Write attempts to write message contents to a target Azure Blob Storage container as files.
func (a *AzureBlobStorage) Write(msg types.Message) error {
	return a.WriteWithContext(context.Background(), msg)
}

func (a *AzureBlobStorage) getContainer(name string) (*azblob.ContainerURL, error) {
	accountName, accountKey := a.conf.StorageAccount, a.conf.StorageAccessKey
	if len(accountName) == 0 || len(accountKey) == 0 {
		return nil, fmt.Errorf("invalid azure storage account credentials")
	}
	credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		return nil, fmt.Errorf("invalid azure storage account credentials: %v", err)
	}
	p := azblob.NewPipeline(credential, azblob.PipelineOptions{})
	URL, _ := url.Parse(fmt.Sprintf("https://%s.blob.core.windows.net/%s", accountName, name))
	containerURL := azblob.NewContainerURL(*URL, p)
	return &containerURL, nil
}

func (a *AzureBlobStorage) uploadToBlob(ctx context.Context, message []byte, blobName string, blobType string, containerURL *azblob.ContainerURL) error {
	var err error

	switch blobType {
	case "BLOCK":
		blobURL := containerURL.NewBlockBlobURL(blobName)
		_, err = azblob.UploadStreamToBlockBlob(ctx, bytes.NewReader(message), blobURL, azblob.UploadStreamToBlockBlobOptions{})
	case "APPEND":
		blobURL := containerURL.NewAppendBlobURL(blobName)
		_, err = blobURL.AppendBlock(ctx, bytes.NewReader(message), azblob.AppendBlobAccessConditions{}, nil)
	}

	return err
}

// WriteWithContext attempts to write message contents to a target storage account as files.
func (a *AzureBlobStorage) WriteWithContext(wctx context.Context, msg types.Message) error {
	ctx, cancel := context.WithTimeout(
		wctx, a.timeout,
	)
	defer cancel()

	return msg.Iter(func(i int, p types.Part) error {
		c, err := a.getContainer(a.container.String(i, msg))
		if err != nil {
			return err
		}
		if err := a.uploadToBlob(ctx, p.Get(), a.path.String(i, msg), a.blobType.String(i, msg), c); err != nil {
			a.log.Errorf("Error uploading blob: %v.", err)
			if containerNotFound(err) {
				if _, cerr := c.Create(ctx, azblob.Metadata{}, azblob.PublicAccessNone); cerr != nil {
					a.log.Errorf("Error creating container: %v.", cerr)
				}
			}
			return err
		}
		return nil
	})
}

func containerNotFound(err error) bool {
	if serr, ok := err.(azblob.StorageError); ok {
		return serr.ServiceCode() == azblob.ServiceCodeContainerNotFound
	}
	// TODO azure blob storage api (preview) is returning an internal wrapped storageError
	return strings.Contains(err.Error(), "ContainerNotFound")
}

// CloseAsync begins cleaning up resources used by this reader asynchronously.
func (a *AzureBlobStorage) CloseAsync() {
}

// WaitForClose will block until either the reader is closed or a specified
// timeout occurs.
func (a *AzureBlobStorage) WaitForClose(time.Duration) error {
	return nil
}

//------------------------------------------------------------------------------
