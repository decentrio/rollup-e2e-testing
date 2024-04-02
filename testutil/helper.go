package testutil

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/decentrio/rollup-e2e-testing/ibc"
	"github.com/stretchr/testify/require"
)

func AssertBalance(t *testing.T, ctx context.Context, chain ibc.Chain, address string, denom string, expectedBalance sdkmath.Int) {
	balance, err := chain.GetBalance(ctx, address, denom)
	require.NoError(t, err)
	require.Equal(t, expectedBalance.String(), balance.String())
}

func CopyDir(src, dst string) error {
	// Check source directory is exists
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("source directory is not exist %s: %s", src, err)
	}
	// Check destination directory is exists, if not create it
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		if err := os.MkdirAll(dst, 0755); err != nil {
			return fmt.Errorf("can not create destination directory %s: %s", dst, err)
		}
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := CopyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return nil
}
