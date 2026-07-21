package prompt

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
	"github.com/lucas-garcia-rubio/fluxos/internal/trace"
)

func TestParseSelectionTokensIsPureAndCaseInsensitive(t *testing.T) {
	got, err := ParseSelectionTokens("2=ALL 1=NoNe 3=07")
	if err != nil {
		t.Fatalf("ParseSelectionTokens: %v", err)
	}
	want := []SelectionToken{
		{Site: 2, Kind: ChoiceTokenAll},
		{Site: 1, Kind: ChoiceTokenNone},
		{Site: 3, Kind: ChoiceTokenCandidate, Candidate: 7},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("tokens = %+v, want %+v", got, want)
	}
}

func TestParseSelectionTokensRejectsMalformedAndDuplicateTokens(t *testing.T) {
	for _, input := range []string{
		"",
		"1",
		"1=",
		"=1",
		"0=1",
		"1=0",
		"1=maybe",
		"1=2 1=none",
		"1 = 2",
	} {
		t.Run(strings.ReplaceAll(input, " ", "_"), func(t *testing.T) {
			if _, err := ParseSelectionTokens(input); err == nil {
				t.Fatalf("ParseSelectionTokens(%q): expected error", input)
			}
		})
	}
}

func TestParseSelectionTokensErrorsUsePortuguese(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: "a seleção está vazia"},
		{input: "1", want: "formato site=opção"},
		{input: "0=1", want: "local \"0\" inválido"},
		{input: "1=0", want: "escolha \"0\" inválida"},
		{input: "1=2 1=none", want: "mais de uma vez"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			_, err := ParseSelectionTokens(test.input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ParseSelectionTokens(%q) error = %v, want Portuguese fragment %q", test.input, err, test.want)
			}
		})
	}
}

func TestPickerKeepsPromptOffStdoutAndPreservesCandidateOrder(t *testing.T) {
	sites := testSites()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	picker := NewPicker(strings.NewReader("1=1\n"), &stderr, 5)

	got, err := picker.Select(sites[:1])
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(got) != 1 || got[0].Choice.Mode != resolve.ChoiceSelected || got[0].Choice.ImplementationFQCN != "service.First" {
		t.Fatalf("selections = %+v, want first candidate", got)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	output := stderr.String()
	if !strings.Contains(output, "caller.Workflow.start()") ||
		!strings.Contains(output, "src/main/java/Workflow.java:27") ||
		!strings.Contains(output, "contract.Service.run()") ||
		!strings.Contains(output, "1) service.First") ||
		!strings.Contains(output, "2) service.Second") {
		t.Fatalf("prompt output missing required site details:\n%s", output)
	}
	if strings.Index(output, "1) service.First") > strings.Index(output, "2) service.Second") {
		t.Fatalf("candidate order changed:\n%s", output)
	}
	assertPortugueseTranscript(t, output)
}

func TestPickerReaderPersistsAcrossSelectCalls(t *testing.T) {
	var stderr bytes.Buffer
	picker := NewPicker(strings.NewReader("1=1\n1=2\n"), &stderr, 0)
	site := testSites()[:1]

	first, err := picker.Select(site)
	if err != nil {
		t.Fatalf("first Select: %v", err)
	}
	second, err := picker.Select(site)
	if err != nil {
		t.Fatalf("second Select: %v", err)
	}
	if first[0].Choice.ImplementationFQCN != "service.First" || second[0].Choice.ImplementationFQCN != "service.Second" {
		t.Fatalf("choices = %q, %q; reader was not persistent", first[0].Choice.ImplementationFQCN, second[0].Choice.ImplementationFQCN)
	}
	if strings.Count(stderr.String(), "1 local de implementação ambíguo") != 2 {
		t.Fatalf("rendered rounds = %d, want 2", strings.Count(stderr.String(), "1 local de implementação ambíguo"))
	}
}

func TestPickerSelectedNoneAndAllChoices(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxImpls  int
		wantModes []resolve.ChoiceMode
		wantNames []string
		wantText  string
	}{
		{
			name:      "selected",
			input:     "1=2\n",
			maxImpls:  5,
			wantModes: []resolve.ChoiceMode{resolve.ChoiceSelected},
			wantNames: []string{"service.Second"},
		},
		{
			name:      "none",
			input:     "1=none\n",
			maxImpls:  5,
			wantModes: []resolve.ChoiceMode{resolve.ChoiceNone},
		},
		{
			name:      "all with limit",
			input:     "1=all\n",
			maxImpls:  3,
			wantModes: []resolve.ChoiceMode{resolve.ChoiceAll},
			wantText:  "all) all (máximo 3)",
		},
		{
			name:      "all unlimited",
			input:     "1=all\n",
			maxImpls:  0,
			wantModes: []resolve.ChoiceMode{resolve.ChoiceAll},
			wantText:  "all) all (ilimitado)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			got, err := NewPicker(strings.NewReader(test.input), &stderr, test.maxImpls).Select(testSites()[:1])
			if err != nil {
				t.Fatalf("Select: %v", err)
			}
			if len(got) != len(test.wantModes) {
				t.Fatalf("selections = %+v", got)
			}
			for index, selection := range got {
				if selection.Choice.Mode != test.wantModes[index] {
					t.Errorf("selection[%d].Mode = %q, want %q", index, selection.Choice.Mode, test.wantModes[index])
				}
				if len(test.wantNames) > index && selection.Choice.ImplementationFQCN != test.wantNames[index] {
					t.Errorf("selection[%d].ImplementationFQCN = %q, want %q", index, selection.Choice.ImplementationFQCN, test.wantNames[index])
				}
			}
			if test.wantText != "" && !strings.Contains(stderr.String(), test.wantText) {
				t.Errorf("output = %q, want %q", stderr.String(), test.wantText)
			}
		})
	}
}

func TestPickerPartialConfirmationAndAtomicRetry(t *testing.T) {
	sites := testSites()
	var stderr bytes.Buffer
	input := "1=2\nNAO\n1=2 2=all\n"
	got, err := NewPicker(strings.NewReader(input), &stderr, 5).Select(sites)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(got) != 2 || got[0].Choice.ImplementationFQCN != "service.Second" || got[1].Choice.Mode != resolve.ChoiceAll {
		t.Fatalf("selections = %+v, want only final complete batch", got)
	}
	output := stderr.String()
	if !strings.Contains(output, "Confirmar none para os sites não informados [2]? [sim/nao] ") {
		t.Fatalf("partial confirmation/retry output missing:\n%s", stderr.String())
	}
	assertPortugueseTranscript(t, output)
}

func TestPickerPartialConfirmationFillsOmittedSitesWithNone(t *testing.T) {
	var stderr bytes.Buffer
	got, err := NewPicker(strings.NewReader("2=1\nsim\n"), &stderr, 5).Select(testSitesThree())
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(got) != 3 || got[0].Choice.Mode != resolve.ChoiceNone || got[1].Choice.ImplementationFQCN != "service.First" || got[2].Choice.Mode != resolve.ChoiceNone {
		t.Fatalf("selections = %+v, want omitted site none and selected site", got)
	}
	if !strings.Contains(stderr.String(), "Confirmar none para os sites não informados [1, 3]? [sim/nao] ") {
		t.Fatalf("positive partial confirmation omitted-site list missing:\n%s", stderr.String())
	}
	assertPortugueseTranscript(t, stderr.String())
}

func TestPickerInvalidConfirmationRetriesThenNegativeReturnsToBatch(t *testing.T) {
	var stderr bytes.Buffer
	input := "1=2\ntalvez\nNÃO\n1=2 2=all\n"
	got, err := NewPicker(strings.NewReader(input), &stderr, 5).Select(testSites())
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(got) != 2 || got[0].Choice.ImplementationFQCN != "service.Second" || got[1].Choice.Mode != resolve.ChoiceAll {
		t.Fatalf("selections = %+v, want only final complete batch", got)
	}
	output := stderr.String()
	confirmation := "Confirmar none para os sites não informados [2]? [sim/nao] "
	if strings.Count(output, confirmation) != 2 {
		t.Fatalf("confirmation prompt count = %d, want 2:\n%s", strings.Count(output, confirmation), output)
	}
	if !strings.Contains(output, "resposta inválida: informe sim/s/yes/y para confirmar ou nao/não/n para recusar.") {
		t.Fatalf("invalid confirmation response missing:\n%s", output)
	}
	assertPortugueseTranscript(t, output)
}

func TestPickerBlankAndInvalidInputRetry(t *testing.T) {
	var stderr bytes.Buffer
	got, err := NewPicker(strings.NewReader("\n1=99\n1=1\n"), &stderr, 5).Select(testSites()[:1])
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got[0].Choice.ImplementationFQCN != "service.First" {
		t.Fatalf("choice = %+v, want service.First", got[0].Choice)
	}
	output := stderr.String()
	if !strings.Contains(output, "ajuda: informe local=opção") ||
		!strings.Contains(output, "o candidato 99 está fora do intervalo para o local 1") {
		t.Fatalf("retry output missing:\n%s", output)
	}
	if strings.Count(output, "[1] caller.Workflow.start()") != 1 {
		t.Fatalf("site was redrawn during retry:\n%s", output)
	}
	assertPortugueseTranscript(t, output)
}

func TestPickerCancellationAndEOFWrapSelectionCanceled(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
	}{
		{name: "q", input: "q\n"},
		{name: "cancel", input: "CaNcEl\n"},
		{name: "eof", input: ""},
		{name: "unterminated partial line", input: "1=1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			_, err := NewPicker(strings.NewReader(test.input), &stderr, 5).Select(testSites()[:1])
			if err != trace.ErrSelectionCanceled || !errors.Is(err, trace.ErrSelectionCanceled) {
				t.Fatalf("Select error = %v, want ErrSelectionCanceled", err)
			}
			if err.Error() != trace.ErrSelectionCanceled.Error() {
				t.Fatalf("cancellation error text = %q, want %q", err.Error(), trace.ErrSelectionCanceled.Error())
			}
		})
	}
}

func TestPickerReaderAndWriterErrorsPropagate(t *testing.T) {
	readErr := errors.New("input failed")
	var stderr bytes.Buffer
	_, err := NewPicker(errorReader{err: readErr}, &stderr, 5).Select(testSites()[:1])
	if !errors.Is(err, readErr) {
		t.Fatalf("reader error = %v, want %v", err, readErr)
	}

	writeErr := errors.New("output failed")
	_, err = NewPicker(strings.NewReader("1=1\n"), errorWriter{err: writeErr}, 5).Select(testSites()[:1])
	if !errors.Is(err, writeErr) {
		t.Fatalf("writer error = %v, want %v", err, writeErr)
	}
}

func TestValidateTokensUsesInputCandidateOrderAndSiteOrder(t *testing.T) {
	sites := testSites()
	tokens, err := ParseSelectionTokens("2=1 1=2")
	if err != nil {
		t.Fatalf("ParseSelectionTokens: %v", err)
	}
	batch, err := validateTokens(tokens, sites)
	if err != nil {
		t.Fatalf("validateTokens: %v", err)
	}
	got := selectionsInSiteOrder(sites, batch.choices)
	if got[0].SiteID != sites[0].ID || got[1].SiteID != sites[1].ID {
		t.Fatalf("site order = %+v, want input site order", got)
	}
	if got[0].Choice.ImplementationFQCN != "service.Second" || got[1].Choice.ImplementationFQCN != "service.First" {
		t.Fatalf("candidate choices = %+v", got)
	}
}

func testSites() []resolve.DispatchSite {
	return []resolve.DispatchSite{
		{
			ID: "site-one",
			Caller: resolve.ExecutionKey{Method: resolve.MethodHandle{
				TypeFQCN: "caller.Workflow", Method: "start", Signature: "()",
			}},
			ReceiverFQCN: "contract.Service",
			Method:       "run",
			Signature:    "()",
			Call:         java.CallSite{File: "src/main/java/Workflow.java", Line: 27},
			Candidates: []resolve.ImplementationCandidate{
				{ImplementationFQCN: "service.First"},
				{ImplementationFQCN: "service.Second"},
			},
		},
		{
			ID: "site-two",
			Caller: resolve.ExecutionKey{Method: resolve.MethodHandle{
				TypeFQCN: "caller.Workflow", Method: "finish", Signature: "()",
			}},
			ReceiverFQCN: "contract.OtherService",
			Method:       "close",
			Signature:    "()",
			Call:         java.CallSite{File: "src/main/java/Workflow.java", Line: 31},
			Candidates: []resolve.ImplementationCandidate{
				{ImplementationFQCN: "service.First"},
				{ImplementationFQCN: "service.Second"},
			},
		},
	}
}

func testSitesThree() []resolve.DispatchSite {
	sites := testSites()
	third := sites[1]
	third.ID = "site-three"
	sites = append(sites, third)
	return sites
}

func assertPortugueseTranscript(t *testing.T, output string) {
	t.Helper()
	for _, forbidden := range []string{
		"ambiguous",
		"implementation",
		"choices",
		"example",
		"keep terminal",
		"unlimited",
		"help:",
		"invalid selection",
		"no choices applied",
		"out of range",
	} {
		if strings.Contains(strings.ToLower(output), forbidden) {
			t.Errorf("transcript contains untranslated English %q:\n%s", forbidden, output)
		}
	}
}

type errorReader struct {
	err error
}

func (reader errorReader) Read([]byte) (int, error) {
	return 0, reader.err
}

type errorWriter struct {
	err error
}

func (writer errorWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

var _ trace.ImplementationSelector = (*Picker)(nil)
