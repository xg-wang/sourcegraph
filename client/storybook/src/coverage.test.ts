import path from 'path'
import { pathToFileURL } from 'url'

import initStoryshots from '@storybook/addon-storyshots'
import { puppeteerTest } from '@storybook/addon-storyshots-puppeteer'

import { recordCoverage } from '@sourcegraph/shared/src/testing/coverage'
import { getPuppeteerBrowser } from '@sourcegraph/shared/src/testing/driver'
import { PUPPETEER_BROWSER_REVISION } from '@sourcegraph/shared/src/testing/puppeteer-browser-revision'

// This test suite does not actually test anything.
// It just loads up the storybook in Puppeteer and records its coverage,
// so it can be tracked in Codecov.

initStoryshots({
    configPath: __dirname,
    suite: 'Storybook',
    test: puppeteerTest({
        chromeExecutablePath: getPuppeteerBrowser('chrome', PUPPETEER_BROWSER_REVISION.chrome).executablePath,
        storybookUrl: pathToFileURL(path.resolve(__dirname, '../storybook-static')).href,
        testBody: async page => {
            await recordCoverage(page.browser())
        },
    }),
})
