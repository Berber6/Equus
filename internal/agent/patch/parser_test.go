package patch

import "testing"

func TestParseAddFile(t *testing.T) {
	hunks, err := Parse("*** Begin Patch\n*** Add File: foo\n+hi\n*** End Patch\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(hunks) != 1 {
		t.Fatalf("hunks = %d", len(hunks))
	}
	add, ok := hunks[0].(*AddFile)
	if !ok || add.FilePath != "foo" || add.Contents != "hi\n" {
		t.Fatalf("hunk = %+v", hunks[0])
	}
}

func TestParseRejectsMissingBegin(t *testing.T) {
	if _, err := Parse("bad\n"); err == nil {
		t.Fatal("expected error for missing begin marker")
	}
}

func TestParseRejectsMissingEnd(t *testing.T) {
	if _, err := Parse("*** Begin Patch\n*** Add File: foo\n+hi\n"); err == nil {
		t.Fatal("expected error for missing end marker")
	}
}

func TestParseRejectsUnknownHeader(t *testing.T) {
	if _, err := Parse("*** Begin Patch\n*** Bogus File: foo\n*** End Patch\n"); err == nil {
		t.Fatal("expected error for unknown header")
	}
}

func TestParseEmptyPatch(t *testing.T) {
	hunks, err := Parse("*** Begin Patch\n*** End Patch\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(hunks) != 0 {
		t.Fatalf("hunks = %d, want 0", len(hunks))
	}
}

func TestParseUpdateWithMove(t *testing.T) {
	patch := "*** Begin Patch\n" +
		"*** Update File: path/update.py\n" +
		"*** Move to: path/update2.py\n" +
		"@@ def f():\n" +
		"-    pass\n" +
		"+    return 123\n" +
		"*** End Patch"
	hunks, err := Parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	update, ok := hunks[0].(*UpdateFile)
	if !ok {
		t.Fatalf("hunk = %T", hunks[0])
	}
	if update.FilePath != "path/update.py" || update.MovePath != "path/update2.py" {
		t.Fatalf("paths = %q -> %q", update.FilePath, update.MovePath)
	}
	if len(update.Chunks) != 1 {
		t.Fatalf("chunks = %d", len(update.Chunks))
	}
	chunk := update.Chunks[0]
	if chunk.ChangeContext != "def f():" || len(chunk.OldLines) != 1 || chunk.OldLines[0] != "    pass" {
		t.Fatalf("chunk = %+v", chunk)
	}
	if len(chunk.NewLines) != 1 || chunk.NewLines[0] != "    return 123" {
		t.Fatalf("new lines = %+v", chunk.NewLines)
	}
}

func TestParseImplicitChunkWithoutContext(t *testing.T) {
	patch := "*** Begin Patch\n*** Update File: file2.py\n import foo\n+bar\n*** End Patch"
	hunks, err := Parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	update := hunks[0].(*UpdateFile)
	chunk := update.Chunks[0]
	if chunk.ChangeContext != "" {
		t.Fatalf("context = %q", chunk.ChangeContext)
	}
	if len(chunk.OldLines) != 1 || chunk.OldLines[0] != "import foo" {
		t.Fatalf("old = %+#v", chunk.OldLines)
	}
	if len(chunk.NewLines) != 2 || chunk.NewLines[1] != "bar" {
		t.Fatalf("new = %+#v", chunk.NewLines)
	}
	if len(chunk.ContextIndices) != 1 {
		t.Fatalf("context indices = %+v", chunk.ContextIndices)
	}
}

func TestParseEndOfFileMarker(t *testing.T) {
	patch := "*** Begin Patch\n*** Update File: file.txt\n@@\n+quux\n*** End of File\n\n*** End Patch"
	hunks, err := Parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	chunk := hunks[0].(*UpdateFile).Chunks[0]
	if !chunk.EndOfFile {
		t.Fatal("expected EndOfFile chunk")
	}
}

func TestParseRejectsEmptyUpdateHunk(t *testing.T) {
	if _, err := Parse("*** Begin Patch\n*** Update File: test.py\n*** End Patch"); err == nil {
		t.Fatal("expected error for empty update hunk")
	}
}
