/**
 * Constructs the bundled MOQ worker. Isolated in its own module so the
 * `?worker&inline` import (which pulls the whole worker bundle into the
 * importer) is not dragged into unit tests of the pure RPC transport.
 *
 * `?worker&inline` makes Vite emit the worker as a self-contained Blob
 * inside the library bundle: no sibling file to ship, no URL for a
 * consumer's bundler to resolve or mangle.
 */
import MoqWorker from './moq-worker.ts?worker&inline';

/** The MOQ worker, inlined into the library bundle. */
export function createMoqWorker(): Worker {
  return new MoqWorker();
}
