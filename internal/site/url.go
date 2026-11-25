// Package site renders the static index and its JSON API.
//
// Pages are files. There is no server. The public address is BaseURL.
package site

// BaseURL is the GitHub Pages address of this index.
const BaseURL = "https://stageproof.github.io/index/"

// BadgeWorkerURL is the badge query address. GitHub Pages cannot route that query.
const BadgeWorkerURL = "https://stageproof-index-badges.stageproof.workers.dev/badge.svg"
