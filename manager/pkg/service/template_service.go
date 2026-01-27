package service

import (
	"context"
	"fmt"

	"github.com/sandbox0-ai/infra/manager/pkg/apis/sandbox0/v1alpha1"
	"github.com/sandbox0-ai/infra/manager/pkg/controller"
	clientset "github.com/sandbox0-ai/infra/manager/pkg/generated/clientset/versioned"
	"github.com/sandbox0-ai/infra/manager/pkg/network"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TemplateService handles template operations
type TemplateService struct {
	crdClient         clientset.Interface
	templateLister    controller.TemplateLister
	logger            *zap.Logger
	network           network.Provider
	templateNamespace string
}

// NewTemplateService creates a new TemplateService
func NewTemplateService(
	crdClient clientset.Interface,
	templateLister controller.TemplateLister,
	networkProvider network.Provider,
	logger *zap.Logger,
	templateNamespace string,
) *TemplateService {
	if networkProvider == nil {
		networkProvider = network.NewNoopProvider()
	}
	if templateNamespace == "" {
		templateNamespace = "sandbox0"
	}
	return &TemplateService{
		crdClient:         crdClient,
		templateLister:    templateLister,
		logger:            logger,
		network:           networkProvider,
		templateNamespace: templateNamespace,
	}
}

// CreateTemplate creates a new template
func (s *TemplateService) CreateTemplate(ctx context.Context, template *v1alpha1.SandboxTemplate) (*v1alpha1.SandboxTemplate, error) {
	s.logger.Info("Creating template", zap.String("name", template.Name))

	template.Namespace = s.templateNamespace

	if s.network != nil {
		if err := s.network.EnsureBaseline(ctx, s.templateNamespace); err != nil {
			s.logger.Warn("Network provider baseline failed",
				zap.String("provider", s.network.Name()),
				zap.String("namespace", s.templateNamespace),
				zap.Error(err),
			)
		}
	}

	// Set default values if needed
	if template.Spec.Pool.MinIdle < 0 {
		template.Spec.Pool.MinIdle = 0
	}
	if template.Spec.Pool.MaxIdle < template.Spec.Pool.MinIdle {
		template.Spec.Pool.MaxIdle = template.Spec.Pool.MinIdle
	}

	result, err := s.crdClient.Sandbox0V1alpha1().SandboxTemplates(s.templateNamespace).Create(ctx, template, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create template: %w", err)
	}

	return result, nil
}

// GetTemplate gets a template by ID (name) from the configured namespace.
func (s *TemplateService) GetTemplate(ctx context.Context, id string) (*v1alpha1.SandboxTemplate, error) {
	template, err := s.templateLister.Get(s.templateNamespace, id)
	if err != nil {
		return nil, err
	}
	return template, nil
}

// ListTemplates lists templates in the configured namespace.
func (s *TemplateService) ListTemplates(ctx context.Context) ([]*v1alpha1.SandboxTemplate, error) {
	templates, err := s.templateLister.List()
	if err != nil {
		return nil, err
	}

	var filtered []*v1alpha1.SandboxTemplate
	for _, t := range templates {
		if t.Namespace == s.templateNamespace {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

// UpdateTemplate updates an existing template
func (s *TemplateService) UpdateTemplate(ctx context.Context, template *v1alpha1.SandboxTemplate) (*v1alpha1.SandboxTemplate, error) {
	s.logger.Info("Updating template", zap.String("name", template.Name))

	template.Namespace = s.templateNamespace

	// Helper to get current version for optimistic locking
	current, err := s.crdClient.Sandbox0V1alpha1().SandboxTemplates(s.templateNamespace).Get(ctx, template.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get current template: %w", err)
	}

	template.ResourceVersion = current.ResourceVersion

	// Preserve status
	template.Status = current.Status

	result, err := s.crdClient.Sandbox0V1alpha1().SandboxTemplates(s.templateNamespace).Update(ctx, template, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("update template: %w", err)
	}

	return result, nil
}

// DeleteTemplate deletes a template from the configured namespace.
func (s *TemplateService) DeleteTemplate(ctx context.Context, id string) error {
	s.logger.Info("Deleting template", zap.String("name", id))

	err := s.crdClient.Sandbox0V1alpha1().SandboxTemplates(s.templateNamespace).Delete(ctx, id, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("delete template: %w", err)
	}

	return nil
}

// WarmPool triggers pool warming for a template in the configured namespace.
func (s *TemplateService) WarmPool(ctx context.Context, id string, count int32) error {
	s.logger.Info("Warming pool", zap.String("name", id), zap.Int32("count", count))

	// Get current template
	template, err := s.crdClient.Sandbox0V1alpha1().SandboxTemplates(s.templateNamespace).Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get template: %w", err)
	}

	// Update MinIdle if needed
	if template.Spec.Pool.MinIdle < count {
		template.Spec.Pool.MinIdle = count
		if template.Spec.Pool.MaxIdle < count {
			template.Spec.Pool.MaxIdle = count
		}

		_, err = s.crdClient.Sandbox0V1alpha1().SandboxTemplates(s.templateNamespace).Update(ctx, template, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update template pool settings: %w", err)
		}
	}

	return nil
}
