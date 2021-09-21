package search

import (
	"strings"
	"testing"

	"github.com/sourcegraph/go-diff/diff"
	"github.com/stretchr/testify/require"
	// "github.com/sourcegraph/sourcegraph/internal/gitserver/protocol"
)

func TestDiffSearch(t *testing.T) {
	rawDiff := `diff --git a/web/src/integration/gqlresponses/user_settings_bla_response_1.ts b/web/src/integration/gqlresponses/user_settings_bla_response_1.ts
new file mode 100644
index 0000000000..4f6e758628
--- /dev/null
+++ b/web/src/integration/gqlresponses/user_settings_bla_response_1.ts
@@ -0,0 +1,4 @@
+export const overrideSettingsResponse: OverrideSettingsResponseShape = {
+    foo: 1,
+    bar: {},
+}
diff --git a/web/src/integration/helpers.ts b/web/src/integration/helpers.ts
index 2f71392b2f..d874527291 100644
--- a/web/src/integration/helpers.ts
+++ b/web/src/integration/helpers.ts
@@ -5,7 +5,7 @@ import { createDriverForTest, Driver } from '../../../shared/src/testing/driver'
 import * as path from 'path'
 import mkdirp from 'mkdirp-promise'
 import express from 'express'
-import { Polly } from '@pollyjs/core'
+import { Polly, Request, Response } from '@pollyjs/core'
 import { PuppeteerAdapter } from './polly/PuppeteerAdapter'
 import FSPersister from '@pollyjs/persister-fs'
`
	r := diff.NewMultiFileDiffReader(strings.NewReader(rawDiff))
	_, err := r.ReadAllFiles()
	require.NoError(t, err)
}
