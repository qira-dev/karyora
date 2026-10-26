// Package plugins owns the plugin ABI host contracts and permission/resource
// policies. Third-party plugin execution stays DISABLED until the D-14
// runtime evidence gate passes; this package must not enable untrusted
// execution, and no instruction fuel-metering claim is made without
// documented runtime support.
package plugins
