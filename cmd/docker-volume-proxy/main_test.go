/*
 * Copyright Datadatdat.
 */

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_NoArgsErrors(t *testing.T) {
	// No socket path -> NArg() == 0, run returns "missing required socket path".
	var out, errOut bytes.Buffer
	err := run([]string{}, &out, &errOut)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required socket path")
	assert.Empty(t, out.String(), "should not print 'Proxying' banner when args invalid")
}

func TestRun_TooManyPositionalArgsErrors(t *testing.T) {
	// Two socket paths -> NArg() == 2, run returns "missing required socket path"
	// (same branch — NArg != 1).
	var out, errOut bytes.Buffer
	err := run([]string{"/tmp/one.sock", "/tmp/two.sock"}, &out, &errOut)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required socket path")
}

func TestRun_InvalidFlagErrors(t *testing.T) {
	// Unknown flag -> fs.Parse returns a parse error. Flag.ContinueOnError
	// surfaces it as the return value rather than os.Exit.
	var out, errOut bytes.Buffer
	err := run([]string{"--bogus-flag", "/tmp/a.sock"}, &out, &errOut)
	require.Error(t, err)
	// FlagSet writes its own diagnostic + usage to stderr; just confirm it ran.
	assert.NotEmpty(t, errOut.String())
}

func TestRun_NonIntPortErrors(t *testing.T) {
	// --port expects an int; passing a non-numeric value is a parse error.
	var out, errOut bytes.Buffer
	err := run([]string{"--port", "not-a-number", "/tmp/a.sock"}, &out, &errOut)
	require.Error(t, err)
}

func TestRun_ValidArgsReachListen(t *testing.T) {
	// Use a guaranteed-bad socket path so Listen() fails quickly with a
	// "listen failed on" error — that proves we reached the bottom of run()
	// (flag parsing succeeded, banner printed, forwarder + listener built).
	var out, errOut bytes.Buffer
	err := run(
		[]string{"--host", "127.0.0.1", "--port", "9999", "/nonexistent/deeply/nested/dir/test.sock"},
		&out, &errOut,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listen failed on")
	// The banner is printed before listener construction, so it must appear
	// in stdout regardless of the Listen() failure.
	assert.True(t, strings.HasPrefix(out.String(), "Proxying requests from /nonexistent"),
		"banner missing: %q", out.String())
	assert.Contains(t, out.String(), "127.0.0.1:9999")
}

func TestRun_DefaultHostPortShownInBanner(t *testing.T) {
	// Confirms the default flag values (localhost / 5001) flow through to
	// the banner when not overridden.
	var out, errOut bytes.Buffer
	_ = run([]string{"/nonexistent/sock"}, &out, &errOut)
	assert.Contains(t, out.String(), "localhost:5001")
}
