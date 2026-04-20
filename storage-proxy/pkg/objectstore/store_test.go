package objectstore

import (
	"bytes"
	"io"
	"testing"
)

func TestBuildEndpointGCS(t *testing.T) {
	storageType, endpoint, err := BuildEndpoint(Config{Type: "gcs", Bucket: "sandbox0-data"})
	if err != nil {
		t.Fatalf("BuildEndpoint() error = %v", err)
	}
	if storageType != TypeGCS {
		t.Fatalf("storage type = %q, want %q", storageType, TypeGCS)
	}
	if endpoint != "gs://sandbox0-data" {
		t.Fatalf("endpoint = %q, want gs://sandbox0-data", endpoint)
	}
}

func TestGCSProjectIDPrefersConfigRegion(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "env-project")

	got := gcsProjectID(Config{Region: "config-project"})
	if got != "config-project" {
		t.Fatalf("project id = %q, want config-project", got)
	}
}

func TestGCSProjectIDFallsBackToEnvironment(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "env-project")

	got := gcsProjectID(Config{})
	if got != "env-project" {
		t.Fatalf("project id = %q, want env-project", got)
	}
}

func TestGCSBaseURLUsesConfiguredEndpoint(t *testing.T) {
	got := gcsBaseURL(Config{Endpoint: "https://storage.example.test/"})
	if got != "https://storage.example.test" {
		t.Fatalf("base URL = %q, want https://storage.example.test", got)
	}
}

func TestGCSObjectURLEscapesObjectNameAsSinglePathSegment(t *testing.T) {
	store := &gcsStore{bucket: "sandbox0-data", baseURL: "https://storage.googleapis.com"}

	got := store.objectURL("team-a/volume-a/meta.json")
	want := "https://storage.googleapis.com/storage/v1/b/sandbox0-data/o/team-a%2Fvolume-a%2Fmeta.json"
	if got != want {
		t.Fatalf("object URL = %q, want %q", got, want)
	}
}

func TestCountingReaderForPreservesReadSeeker(t *testing.T) {
	reader := countingReaderFor(bytes.NewReader([]byte("hello")))

	readSeeker, ok := reader.(io.ReadSeeker)
	if !ok {
		t.Fatal("counting reader should preserve io.ReadSeeker")
	}

	buf := make([]byte, 2)
	n, err := readSeeker.Read(buf)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if n != 2 || string(buf) != "he" {
		t.Fatalf("Read() = %d, %q; want 2, he", n, string(buf))
	}
	if reader.BytesRead() != 2 {
		t.Fatalf("BytesRead() = %d, want 2", reader.BytesRead())
	}

	if _, err := readSeeker.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek() error = %v", err)
	}
	n, err = readSeeker.Read(buf)
	if err != nil {
		t.Fatalf("Read() after seek error = %v", err)
	}
	if n != 2 || string(buf) != "he" {
		t.Fatalf("Read() after seek = %d, %q; want 2, he", n, string(buf))
	}
	if reader.BytesRead() != 4 {
		t.Fatalf("BytesRead() after seek = %d, want 4", reader.BytesRead())
	}
}
