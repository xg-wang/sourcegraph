import React from 'react'

import { pluralize } from '@sourcegraph/shared/src/util/strings'

import { ParsedRepoURI } from '../util/url'

import { HighlightedLinkProps } from './components/HighlightedLink'
import { FuzzyFinderProps, FuzzyFSM, Indexing } from './fsm'
import { FuzzySearch, FuzzySearchResult, SearchIndexing, SearchValue } from './FuzzySearch'
import { CaseInsensitiveFuzzySearch } from './search/CaseInsensitiveFuzzySearch'
import { WordSensitiveFuzzySearch } from './search/WordSensitiveFuzzySearch'

// The default value of 80k filenames is picked from the following observations:
// - case-insensitive search is slow but works in the torvalds/linux repo (72k files)
// - case-insensitive search is almost unusable in the chromium/chromium repo (360k files)
const DEFAULT_CASE_INSENSITIVE_FILE_COUNT_THRESHOLD = 80000
// Cache for the last fuzzy query. This value is only used to avoid redoing the
// full fuzzy search on every re-render when the user presses the down/up arrow
// keys to move the "focus index".
const lastFuzzySearchResult = new Map<string, FuzzySearchResult>()

export interface FuzzyModalProps extends FuzzyFinderProps {
    initialMaxResults: number
    initialQuery: string
    downloadFilenames: () => Promise<string[]>

    isVisible: boolean
    onClose: () => void

    fsm: FuzzyFSM
    setFsm: (fsm: FuzzyFSM) => void

    // Parse the URL in the modal instead of accepting it as a React prop because the
    // URL can change based on shortcuts like `y` that won't trigger a re-render
    // in React. By parsing the URL here, we avoid the risk of rendering links to a revision that
    // doesn't match the active revision in the browser's address bar.
    parseRepoUrl: () => ParsedRepoURI
}

export interface FuzzyModalState {
    query: string
    maxResults: number
}

function plural(what: string, count: number, isComplete: boolean): string {
    return `${count.toLocaleString()}${isComplete ? '' : '+'} ${pluralize(what, count)}`
}
interface FuzzyResultsSummaryProps {
    fsm: FuzzyFSM
    files: RenderedFuzzyResult
    className?: string
}

export const FuzzyResultsSummary: React.FunctionComponent<FuzzyResultsSummaryProps> = ({ fsm, files, className }) => (
    <>
        <span className={className}>
            {plural('result', files.resultsCount, files.isComplete)} -{' '}
            {fsm.key === 'indexing' && indexingProgressBar(fsm)} {plural('total file', files.totalFileCount, true)}
        </span>
        <i className="text-muted">
            <kbd>↑</kbd> and <kbd>↓</kbd> arrow keys browse. <kbd>⏎</kbd> selects.
        </i>
    </>
)

function indexingProgressBar(indexing: Indexing): JSX.Element {
    const indexedFiles = indexing.indexing.indexedFileCount
    const totalFiles = indexing.indexing.totalFileCount
    const percentage = Math.round((indexedFiles / totalFiles) * 100)
    return (
        <progress value={indexedFiles} max={totalFiles}>
            {percentage}%
        </progress>
    )
}

interface RenderedFuzzyResult {
    element?: JSX.Element // TODO why can't component do this? Just return list of highlighted links. optional for message display now.
    linksToRender: HighlightedLinkProps[]
    resultsCount: number
    isComplete: boolean
    totalFileCount: number
    elapsedMilliseconds?: number
    falsePositiveRatio?: number
}

export function renderFuzzyResult(
    props: Pick<
        FuzzyModalProps,
        | 'fsm'
        | 'setFsm'
        | 'downloadFilenames'
        | 'parseRepoUrl'
        | 'onClose'
        | 'repoName'
        | 'commitID'
        | 'platformContext'
    >, // TODO: separate type
    state: FuzzyModalState
): RenderedFuzzyResult {
    function empty(element: JSX.Element): RenderedFuzzyResult {
        return {
            element,
            linksToRender: [],
            resultsCount: 0,
            isComplete: true,
            totalFileCount: 0,
        }
    }

    function onError(what: string): (error: Error) => void {
        return error => {
            props.setFsm({ key: 'failed', errorMessage: JSON.stringify(error) })
            throw new Error(what)
        }
    }

    switch (props.fsm.key) {
        case 'empty':
            handleEmpty(props).then(() => {}, onError('onEmpty'))
            return empty(<></>)
        case 'downloading':
            return empty(<p>Downloading...</p>)
        case 'failed':
            return empty(<p>Error: {props.fsm.errorMessage}</p>)
        case 'indexing': {
            const loader = props.fsm.indexing
            later()
                .then(() => continueIndexing(loader))
                .then(next => props.setFsm(next), onError('onIndexing'))
            return renderFiles({
                props,
                state,
                search: props.fsm.indexing.partialFuzzy,
                indexing: props.fsm.indexing,
                repoUrl: props.parseRepoUrl(),
            })
        }
        case 'ready':
            return renderFiles({
                props,
                state,
                search: props.fsm.fuzzy,
                repoUrl: props.parseRepoUrl(),
            })
        default:
            return empty(<p>ERROR</p>)
    }
}

function renderFiles({
    props,
    state,
    search,
    indexing,
    repoUrl,
}: {
    props: Pick<FuzzyModalProps, 'onClose' | 'repoName' | 'commitID' | 'platformContext'>
    state: FuzzyModalState
    search: FuzzySearch
    indexing?: SearchIndexing
    repoUrl: ParsedRepoURI
}): RenderedFuzzyResult {
    const indexedFileCount = indexing ? indexing.indexedFileCount : ''
    const cacheKey = `${state.query}-${state.maxResults}${indexedFileCount}-${repoUrl.revision || ''}`
    let fuzzyResult = lastFuzzySearchResult.get(cacheKey)
    if (!fuzzyResult) {
        const start = window.performance.now()
        fuzzyResult = search.search({
            query: state.query,
            maxResults: state.maxResults,
            createUrl: filename =>
                props.platformContext.urlToFile(
                    {
                        repoName: props.repoName,
                        revision: repoUrl.revision ?? props.commitID,
                        filePath: filename,
                    },
                    { part: undefined }
                ),
            onClick: () => props.onClose(),
        })
        fuzzyResult.elapsedMilliseconds = window.performance.now() - start
        lastFuzzySearchResult.clear() // Only cache the last query.
        lastFuzzySearchResult.set(cacheKey, fuzzyResult)
    }
    const links = fuzzyResult.links
    if (links.length === 0) {
        return {
            element: <p>No files matching '{state.query}'</p>,
            linksToRender: [],
            resultsCount: 0,
            totalFileCount: search.totalFileCount,
            isComplete: fuzzyResult.isComplete,
        }
    }

    const linksToRender = links.slice(0, state.maxResults)
    return {
        linksToRender,
        resultsCount: linksToRender.length,
        totalFileCount: search.totalFileCount,
        isComplete: fuzzyResult.isComplete,
        elapsedMilliseconds: fuzzyResult.elapsedMilliseconds,
        falsePositiveRatio: fuzzyResult.falsePositiveRatio,
    }
}

async function later(): Promise<void> {
    return new Promise(resolve => setTimeout(() => resolve(), 0))
}

async function continueIndexing(indexing: SearchIndexing): Promise<FuzzyFSM> {
    const next = await indexing.continue()
    if (next.key === 'indexing') {
        return { key: 'indexing', indexing: next }
    }
    return {
        key: 'ready',
        fuzzy: next.value,
    }
}

async function handleEmpty(props: Pick<FuzzyModalProps, 'fsm' | 'setFsm' | 'downloadFilenames'>): Promise<void> {
    props.setFsm({ key: 'downloading' })
    try {
        const filenames = await props.downloadFilenames()
        props.setFsm(handleFilenames(filenames))
    } catch (error) {
        props.setFsm({
            key: 'failed',
            errorMessage: JSON.stringify(error),
        })
    }
    cleanLegacyCacheStorage()
}

function handleFilenames(filenames: string[]): FuzzyFSM {
    const values: SearchValue[] = filenames.map(file => ({ text: file }))
    if (filenames.length < DEFAULT_CASE_INSENSITIVE_FILE_COUNT_THRESHOLD) {
        return {
            key: 'ready',
            fuzzy: new CaseInsensitiveFuzzySearch(values),
        }
    }
    const indexing = WordSensitiveFuzzySearch.fromSearchValuesAsync(values)
    if (indexing.key === 'ready') {
        return {
            key: 'ready',
            fuzzy: indexing.value,
        }
    }
    return {
        key: 'indexing',
        indexing,
    }
}

/**
 * Removes unused cache storage from the initial implementation of the fuzzy finder.
 *
 * This method can be removed in the future. The cache storage was no longer
 * needed after we landed an optimization in the backend that made it faster to
 * download filenames.
 */
function cleanLegacyCacheStorage(): void {
    const cacheAvailable = 'caches' in self
    if (!cacheAvailable) {
        return
    }

    caches.delete('fuzzy-modal').then(
        () => {},
        () => {}
    )
}