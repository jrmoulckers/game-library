// Prettier configuration for the hand-authored browser modules under
// internal/dashboard/static/js/.
//
// The settings are not restated here. They are re-exported from the vendored
// jrmoulckers/engineering configuration, which is fetched byte-identical at the
// ref in .engineering-ref and hash-locked in engineering-configs.lock.json.
// Copying the values instead would fork them, and a fork of a shared config is
// indistinguishable from the original until the day it silently is not.
//
// The .mjs extension is load-bearing. It makes this file ESM regardless of the
// root package.json, which has no "type" field on purpose: adding one would
// change how Node interprets every .js file in the repository, including the
// six browser modules this exists to check.
export { default } from './config/engineering/prettier-config/index.js';
