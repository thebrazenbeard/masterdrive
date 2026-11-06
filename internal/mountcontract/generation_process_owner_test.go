package mountcontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGenerationJournalOwnerChild(t *testing.T) {
	path := os.Getenv("MASTERDRIVE_TEST_JOURNAL_CHILD")
	if path == "" {
		return
	}
	j, err := OpenGenerationJournal(path)
	if os.Getenv("MASTERDRIVE_TEST_EXPECT_DENIAL") == "1" {
		if err == nil {
			j.Close()
			t.Fatal("child acquired journal while parent owned it")
		}
		return
	}
	if err != nil {
		t.Fatalf("child could not reopen released journal: %v", err)
	}
	defer j.Close()
	if err := j.Observe(8); err != nil {
		t.Fatalf("child could not advance journal: %v", err)
	}
}

func runJournalChild(t *testing.T, path string, deny bool) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestGenerationJournalOwnerChild$")
	cmd.Env = append(os.Environ(), "MASTERDRIVE_TEST_JOURNAL_CHILD="+path)
	if deny {
		cmd.Env = append(cmd.Env, "MASTERDRIVE_TEST_EXPECT_DENIAL=1")
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child failed: %v\n%s", err, output)
	}
}

func TestGenerationJournalOwnerAcrossProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal")
	owner, err := OpenGenerationJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Observe(7); err != nil {
		t.Fatal(err)
	}
	runJournalChild(t, path, true)
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	runJournalChild(t, path, false)
	reopened, err := OpenGenerationJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.Observe(7); err == nil {
		t.Fatal("journal accepted rollback after child process advanced generation")
	}
}
