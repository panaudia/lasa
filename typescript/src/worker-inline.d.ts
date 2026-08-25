// Type for Vite's `?worker&inline` import: the default export is a Worker
// constructor backed by an inlined Blob.
declare module '*?worker&inline' {
  const WorkerFactory: new () => Worker;
  export default WorkerFactory;
}
