import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ConfirmationModal } from './ConfirmationModal';
import { EditRouteModal } from './EditRouteModal';

/**
 * Specialized Modals Test Suite
 *
 * Tests for ConfirmationModal and EditRouteModal components
 */

describe('ConfirmationModal', () => {
  describe('Rendering', () => {
    it('should not render when isOpen is false', () => {
      render(
        <ConfirmationModal
          isOpen={false}
          onClose={() => {}}
          onConfirm={() => {}}
          title="Delete Route"
          message="Are you sure?"
        />
      );

      expect(screen.queryByText('Delete Route')).not.toBeInTheDocument();
    });

    it('should render title and message when isOpen is true', () => {
      render(
        <ConfirmationModal
          isOpen={true}
          onClose={() => {}}
          onConfirm={() => {}}
          title="Delete Route"
          message="Are you sure you want to delete this route?"
        />
      );

      expect(screen.getByText('Delete Route')).toBeInTheDocument();
      expect(screen.getByText('Are you sure you want to delete this route?')).toBeInTheDocument();
    });
  });

  describe('Actions', () => {
    it('should call onConfirm when confirm button is clicked', async () => {
      const user = userEvent.setup();
      const onConfirm = vi.fn();

      render(
        <ConfirmationModal
          isOpen={true}
          onClose={() => {}}
          onConfirm={onConfirm}
          title="Confirm Action"
          message="Are you sure?"
        />
      );

      const confirmButton = screen.getByRole('button', { name: /confirm/i });
      await user.click(confirmButton);

      expect(onConfirm).toHaveBeenCalledTimes(1);
    });

    it('should call onClose when cancel button is clicked', async () => {
      const user = userEvent.setup();
      const onClose = vi.fn();

      render(
        <ConfirmationModal
          isOpen={true}
          onClose={onClose}
          onConfirm={() => {}}
          title="Confirm Action"
          message="Are you sure?"
        />
      );

      const cancelButton = screen.getByRole('button', { name: /cancel/i });
      await user.click(cancelButton);

      expect(onClose).toHaveBeenCalledTimes(1);
    });
  });

  describe('Danger Mode', () => {
    it('should apply danger class when danger prop is true', () => {
      render(
        <ConfirmationModal
          isOpen={true}
          onClose={() => {}}
          onConfirm={() => {}}
          title="Delete Route"
          message="This is dangerous"
          danger={true}
        />
      );

      const confirmButton = screen.getByRole('button', { name: /confirm/i });
      // Component uses btn-error for danger mode
      expect(confirmButton).toHaveClass('btn-error');
    });

    it('should use primary variant when danger is false', () => {
      render(
        <ConfirmationModal
          isOpen={true}
          onClose={() => {}}
          onConfirm={() => {}}
          title="Confirm Action"
          message="This is safe"
          danger={false}
        />
      );

      const confirmButton = screen.getByRole('button', { name: /confirm/i });
      expect(confirmButton).toHaveClass('btn-primary');
    });
  });
});

describe('EditRouteModal', () => {
  const mockOnSave = vi.fn();
  const mockOnClose = vi.fn();

  // Route structure matching actual implementation
  const existingRoute = {
    routeId: 'route-001',
    appId: 'grafana',
    pathBase: '/grafana',
    to: 'http://localhost:3000',
    healthCheck: '/api/health',
    scopes: ['READ', 'WRITE']
  };

  beforeEach(() => {
    mockOnSave.mockClear();
    mockOnClose.mockClear();
  });

  describe('Rendering - Add Mode', () => {
    it('should render add mode title when route is null', () => {
      render(
        <EditRouteModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
          route={null}
        />
      );

      // Check for add mode indicators - title says "ADD NEW ROUTE"
      expect(screen.getByText('ADD NEW ROUTE')).toBeInTheDocument();
    });

    it('should render form fields', () => {
      render(
        <EditRouteModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
          route={null}
        />
      );

      // Check for actual field labels used in the component
      expect(screen.getByText('APPLICATION')).toBeInTheDocument();
      expect(screen.getByText(/PATH/i)).toBeInTheDocument();
      expect(screen.getByText(/TARGET/i)).toBeInTheDocument();
    });
  });

  describe('Rendering - Edit Mode', () => {
    it('should render edit mode title when route is provided', () => {
      render(
        <EditRouteModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
          route={existingRoute}
        />
      );

      // Title says "EDIT ROUTE"
      expect(screen.getByText('EDIT ROUTE')).toBeInTheDocument();
    });

    it('should populate form with existing route data', () => {
      render(
        <EditRouteModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
          route={existingRoute}
        />
      );

      // Check that form inputs are populated
      const appIdInput = screen.getByDisplayValue('grafana');
      expect(appIdInput).toBeInTheDocument();

      const pathInput = screen.getByDisplayValue('/grafana');
      expect(pathInput).toBeInTheDocument();
    });
  });

  describe('Cancel Action', () => {
    it('should call onClose when cancel button is clicked', async () => {
      const user = userEvent.setup();

      render(
        <EditRouteModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
          route={null}
        />
      );

      const cancelButton = screen.getByRole('button', { name: /cancel/i });
      await user.click(cancelButton);

      expect(mockOnClose).toHaveBeenCalledTimes(1);
      expect(mockOnSave).not.toHaveBeenCalled();
    });
  });

  describe('Not Rendered When Closed', () => {
    it('should not render when isOpen is false', () => {
      render(
        <EditRouteModal
          isOpen={false}
          onClose={mockOnClose}
          onSave={mockOnSave}
          route={null}
        />
      );

      expect(screen.queryByText('APPLICATION')).not.toBeInTheDocument();
    });
  });
});
