// Package egress builds the HTTP clients that DocsClaw uses for outbound
// traffic inside an OpenShell sandbox.
//
// OpenShell mediates egress transparently: the sandbox runs in its own
// network namespace and every connection is routed through the supervisor's
// proxy, while ambient proxy variables are stripped from the workload. A
// stock http.Transport therefore reaches the supervisor with no code change,
// and no custom dialer may be used.
//
// Clients still keep Proxy: http.ProxyFromEnvironment so that an operator
// who configures an upstream corporate proxy on the supervisor keeps
// working. The source-scan test in this package rejects raw http.Transport
// literals and custom dialers elsewhere in the tree.
package egress
