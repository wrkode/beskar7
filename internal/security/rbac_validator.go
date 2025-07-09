package security

import (
	"context"
	"fmt"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/kubernetes"
)

// RBACValidator provides RBAC security validation functionality
type RBACValidator struct {
	client kubernetes.Interface
}

// NewRBACValidator creates a new RBAC validator
func NewRBACValidator(client kubernetes.Interface) *RBACValidator {
	return &RBACValidator{
		client: client,
	}
}

// RBACValidationResult contains the result of RBAC validation
type RBACValidationResult struct {
	Valid            bool
	Warnings         []string
	Errors           []string
	OverlyBroadRules []OverlyBroadRule
	SecurityFindings []SecurityFinding
}

// OverlyBroadRule represents a rule that grants overly broad permissions
type OverlyBroadRule struct {
	RuleName    string
	APIGroups   []string
	Resources   []string
	Verbs       []string
	Reason      string
	Severity    string
	Suggestions []string
}

// SecurityFinding represents a security issue found in RBAC configuration
type SecurityFinding struct {
	Type        string
	Severity    string
	Description string
	Resource    string
	Rule        string
	Remediation string
}

// ValidateClusterRole validates a ClusterRole for security issues
func (v *RBACValidator) ValidateClusterRole(ctx context.Context, clusterRole *rbacv1.ClusterRole) *RBACValidationResult {
	result := &RBACValidationResult{
		Warnings:         make([]string, 0),
		Errors:           make([]string, 0),
		OverlyBroadRules: make([]OverlyBroadRule, 0),
		SecurityFindings: make([]SecurityFinding, 0),
	}

	for i, rule := range clusterRole.Rules {
		v.validateRule(result, fmt.Sprintf("Rule %d", i), rule)
	}

	// Check for overly broad permissions
	v.checkOverlyBroadPermissions(result, clusterRole)

	// Check for dangerous combinations
	v.checkDangerousPermissions(result, clusterRole)

	// Determine overall validity
	result.Valid = len(result.Errors) == 0

	return result
}

// ValidateRole validates a Role for security issues
func (v *RBACValidator) ValidateRole(ctx context.Context, role *rbacv1.Role) *RBACValidationResult {
	result := &RBACValidationResult{
		Warnings:         make([]string, 0),
		Errors:           make([]string, 0),
		OverlyBroadRules: make([]OverlyBroadRule, 0),
		SecurityFindings: make([]SecurityFinding, 0),
	}

	for i, rule := range role.Rules {
		v.validateRule(result, fmt.Sprintf("Rule %d", i), rule)
	}

	// Determine overall validity
	result.Valid = len(result.Errors) == 0

	return result
}

// validateRule validates a single RBAC rule
func (v *RBACValidator) validateRule(result *RBACValidationResult, ruleName string, rule rbacv1.PolicyRule) {
	// Check for wildcard permissions
	if v.containsWildcard(rule.APIGroups) && v.containsWildcard(rule.Resources) && v.containsWildcard(rule.Verbs) {
		result.OverlyBroadRules = append(result.OverlyBroadRules, OverlyBroadRule{
			RuleName:  ruleName,
			APIGroups: rule.APIGroups,
			Resources: rule.Resources,
			Verbs:     rule.Verbs,
			Reason:    "Grants wildcard permissions (*) for all API groups, resources, and verbs",
			Severity:  "CRITICAL",
			Suggestions: []string{
				"Replace wildcard permissions with specific API groups, resources, and verbs",
				"Follow the principle of least privilege",
				"Create separate rules for different operational needs",
			},
		})
	}

	// Check for dangerous verb combinations
	if v.containsVerb(rule.Verbs, "*") {
		result.SecurityFindings = append(result.SecurityFindings, SecurityFinding{
			Type:        "OverlyBroadVerbs",
			Severity:    "HIGH",
			Description: "Rule grants wildcard verb permissions",
			Resource:    strings.Join(rule.Resources, ","),
			Rule:        ruleName,
			Remediation: "Replace '*' with specific verbs needed for operation",
		})
	}

	// Check for create/delete/patch combination on secrets
	if v.containsResource(rule.Resources, "secrets") || v.containsWildcard(rule.Resources) {
		if v.containsVerb(rule.Verbs, "create") && v.containsVerb(rule.Verbs, "delete") && v.containsVerb(rule.Verbs, "patch") {
			result.SecurityFindings = append(result.SecurityFindings, SecurityFinding{
				Type:        "SecretsFullAccess",
				Severity:    "HIGH",
				Description: "Rule grants full access to secrets (create, delete, patch)",
				Resource:    "secrets",
				Rule:        ruleName,
				Remediation: "Limit secret permissions to only what's needed (e.g., get, list, watch for reading)",
			})
		}
	}

	// Check for cluster-admin equivalent permissions
	if v.isClusterAdminEquivalent(rule) {
		result.SecurityFindings = append(result.SecurityFindings, SecurityFinding{
			Type:        "ClusterAdminEquivalent",
			Severity:    "CRITICAL",
			Description: "Rule grants cluster-admin equivalent permissions",
			Resource:    strings.Join(rule.Resources, ","),
			Rule:        ruleName,
			Remediation: "Break down permissions into specific, minimal rules",
		})
	}

	// Check for impersonation permissions
	if v.containsResource(rule.Resources, "users") || v.containsResource(rule.Resources, "groups") || v.containsResource(rule.Resources, "serviceaccounts") {
		if v.containsVerb(rule.Verbs, "impersonate") {
			result.SecurityFindings = append(result.SecurityFindings, SecurityFinding{
				Type:        "ImpersonationRisk",
				Severity:    "HIGH",
				Description: "Rule grants impersonation permissions",
				Resource:    strings.Join(rule.Resources, ","),
				Rule:        ruleName,
				Remediation: "Remove impersonation permissions unless absolutely necessary",
			})
		}
	}
}

// checkOverlyBroadPermissions checks for overly broad permission patterns
func (v *RBACValidator) checkOverlyBroadPermissions(result *RBACValidationResult, clusterRole *rbacv1.ClusterRole) {
	coreResourcesWithWildcard := false
	rbacResourcesWithWildcard := false

	for i, rule := range clusterRole.Rules {
		ruleName := fmt.Sprintf("Rule %d", i)

		// Check for wildcard access to core resources
		if (v.containsAPIGroup(rule.APIGroups, "") || v.containsWildcard(rule.APIGroups)) && v.containsWildcard(rule.Resources) {
			coreResourcesWithWildcard = true
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s grants wildcard access to core Kubernetes resources", ruleName))
		}

		// Check for wildcard access to RBAC resources
		if v.containsAPIGroup(rule.APIGroups, "rbac.authorization.k8s.io") || v.containsWildcard(rule.APIGroups) {
			if v.containsWildcard(rule.Resources) || v.containsResource(rule.Resources, "*") {
				rbacResourcesWithWildcard = true
				result.SecurityFindings = append(result.SecurityFindings, SecurityFinding{
					Type:        "RBACWildcardAccess",
					Severity:    "HIGH",
					Description: "Rule grants wildcard access to RBAC resources",
					Resource:    "rbac.authorization.k8s.io/*",
					Rule:        ruleName,
					Remediation: "Limit RBAC permissions to specific resources and operations",
				})
			}
		}
	}

	if coreResourcesWithWildcard && rbacResourcesWithWildcard {
		result.SecurityFindings = append(result.SecurityFindings, SecurityFinding{
			Type:        "ExcessivePrivileges",
			Severity:    "CRITICAL",
			Description: "ClusterRole has both core resource and RBAC wildcard access",
			Resource:    clusterRole.Name,
			Rule:        "Multiple rules",
			Remediation: "Split into multiple roles with minimal required permissions",
		})
	}
}

// checkDangerousPermissions checks for dangerous permission combinations
func (v *RBACValidator) checkDangerousPermissions(result *RBACValidationResult, clusterRole *rbacv1.ClusterRole) {
	hasSecretAccess := false
	hasNodeAccess := false
	hasPodExec := false

	for i, rule := range clusterRole.Rules {
		ruleName := fmt.Sprintf("Rule %d", i)

		// Check for secret access
		if v.containsResource(rule.Resources, "secrets") || v.containsWildcard(rule.Resources) {
			hasSecretAccess = true
		}

		// Check for node access
		if v.containsResource(rule.Resources, "nodes") || v.containsWildcard(rule.Resources) {
			hasNodeAccess = true
		}

		// Check for pod exec capabilities
		if v.containsResource(rule.Resources, "pods/exec") || v.containsResource(rule.Resources, "pods/attach") {
			hasPodExec = true
			result.SecurityFindings = append(result.SecurityFindings, SecurityFinding{
				Type:        "PodExecAccess",
				Severity:    "MEDIUM",
				Description: "Rule grants pod exec/attach permissions",
				Resource:    "pods/exec, pods/attach",
				Rule:        ruleName,
				Remediation: "Remove exec/attach permissions unless required for debugging",
			})
		}

		// Check for dangerous resource combinations
		if v.containsResource(rule.Resources, "persistentvolumes") && v.containsVerb(rule.Verbs, "create") {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s can create PersistentVolumes, which may allow host filesystem access", ruleName))
		}
	}

	// Check for dangerous combinations
	if hasSecretAccess && hasNodeAccess {
		result.SecurityFindings = append(result.SecurityFindings, SecurityFinding{
			Type:        "SecretNodeAccess",
			Severity:    "HIGH",
			Description: "ClusterRole has both secret and node access",
			Resource:    clusterRole.Name,
			Rule:        "Multiple rules",
			Remediation: "Consider separating secret and node access into different roles",
		})
	}

	if hasSecretAccess && hasPodExec {
		result.SecurityFindings = append(result.SecurityFindings, SecurityFinding{
			Type:        "SecretExecAccess",
			Severity:    "HIGH",
			Description: "ClusterRole has both secret access and pod exec capabilities",
			Resource:    clusterRole.Name,
			Rule:        "Multiple rules",
			Remediation: "Limit either secret access or exec capabilities",
		})
	}
}

// Helper functions

func (v *RBACValidator) containsWildcard(slice []string) bool {
	return v.containsString(slice, "*")
}

func (v *RBACValidator) containsString(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}

func (v *RBACValidator) containsAPIGroup(slice []string, apiGroup string) bool {
	return v.containsString(slice, apiGroup)
}

func (v *RBACValidator) containsResource(slice []string, resource string) bool {
	return v.containsString(slice, resource)
}

func (v *RBACValidator) containsVerb(slice []string, verb string) bool {
	return v.containsString(slice, verb)
}

func (v *RBACValidator) isClusterAdminEquivalent(rule rbacv1.PolicyRule) bool {
	return v.containsWildcard(rule.APIGroups) &&
		v.containsWildcard(rule.Resources) &&
		v.containsWildcard(rule.Verbs)
}

// GetRecommendedPermissions returns recommended minimal permissions for common use cases
func (v *RBACValidator) GetRecommendedPermissions(useCase string) []rbacv1.PolicyRule {
	switch strings.ToLower(useCase) {
	case "beskar7-minimal":
		return []rbacv1.PolicyRule{
			// Infrastructure resources
			{
				APIGroups: []string{"infrastructure.cluster.x-k8s.io"},
				Resources: []string{"physicalhosts", "beskar7machines", "beskar7clusters"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			{
				APIGroups: []string{"infrastructure.cluster.x-k8s.io"},
				Resources: []string{"physicalhosts/status", "beskar7machines/status", "beskar7clusters/status"},
				Verbs:     []string{"get", "update", "patch"},
			},
			// Cluster API resources (read-only)
			{
				APIGroups: []string{"cluster.x-k8s.io"},
				Resources: []string{"clusters", "machines"},
				Verbs:     []string{"get", "list", "watch"},
			},
			// Secrets (read-only for credentials)
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "list", "watch"},
			},
			// Events for logging
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "patch"},
			},
		}
	case "read-only":
		return []rbacv1.PolicyRule{
			{
				APIGroups: []string{"*"},
				Resources: []string{"*"},
				Verbs:     []string{"get", "list", "watch"},
			},
		}
	default:
		return []rbacv1.PolicyRule{}
	}
}
