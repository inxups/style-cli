// Package template parses and renders STYLE template text.
//
// The package only handles template syntax and variable values. It does not
// validate whether a variable is declared or available to a particular rule;
// callers should perform that validation separately.
package template
