// Package compile turns an already validated STYLE package and plan into the
// deterministic runtime index consumed by the future hook engine.
//
// Compilation is deliberately side-effect free. It does not read package
// files from disk: spec.Load supplies the source bytes used for trust hashes.
package compile
