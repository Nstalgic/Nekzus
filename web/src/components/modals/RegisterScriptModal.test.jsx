import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { RegisterScriptModal } from './RegisterScriptModal';

/**
 * RegisterScriptModal Component Test Suite
 *
 * Tests the RegisterScriptModal component including:
 * - Rendering and visibility
 * - Form field validation
 * - Available scripts fetching
 * - Script type detection
 * - Environment variables management
 * - Form submission
 * - Error handling
 */

describe('RegisterScriptModal', () => {
  const mockOnClose = vi.fn();
  const mockOnSave = vi.fn();

  // Mock fetch for available scripts
  const mockAvailableScripts = [
    { path: 'maintenance/backup.sh', type: 'shell' },
    { path: 'deployment/deploy-app', type: 'go_binary' },
    { path: 'monitoring/check-health.py', type: 'python' }
  ];

  // Helper to select from CustomDropdown
  const selectFromDropdown = async (user, labelText, optionValue) => {
    // Find the label and its associated dropdown
    const label = screen.getByText(labelText);
    const formGroup = label.closest('.form-group');
    const dropdownToggle = formGroup.querySelector('.custom-dropdown-toggle');

    // Open dropdown
    await user.click(dropdownToggle);

    // Click the option
    const option = screen.getByText(optionValue);
    await user.click(option);
  };

  beforeEach(() => {
    vi.clearAllMocks();
    global.fetch = vi.fn();
  });

  describe('Rendering', () => {
    it('should not render when isOpen is false', () => {
      render(
        <RegisterScriptModal
          isOpen={false}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      expect(screen.queryByText('REGISTER SCRIPT')).not.toBeInTheDocument();
    });

    it('should render when isOpen is true', async () => {
      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ available: mockAvailableScripts })
      });

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      expect(screen.getByText('REGISTER SCRIPT')).toBeInTheDocument();
    });

    it('should display all form fields', async () => {
      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ available: mockAvailableScripts })
      });

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByLabelText(/SCRIPT NAME/i)).toBeInTheDocument();
        expect(screen.getByText(/SCRIPT PATH/i)).toBeInTheDocument();
        expect(screen.getByLabelText(/CATEGORY/i)).toBeInTheDocument();
        expect(screen.getByLabelText(/DESCRIPTION/i)).toBeInTheDocument();
        expect(screen.getByLabelText(/TIMEOUT SECONDS/i)).toBeInTheDocument();
      });
    });

    it('should show loading state while fetching scripts', () => {
      global.fetch.mockImplementation(() => new Promise(() => {}));

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      expect(screen.getByText(/loading available scripts/i)).toBeInTheDocument();
    });

    it('should show error when fetch fails', async () => {
      global.fetch.mockRejectedValueOnce(new Error('Network error'));

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByText(/failed to load available scripts/i)).toBeInTheDocument();
      });
    });
  });

  describe('Available Scripts Fetching', () => {
    it('should fetch available scripts on mount', async () => {
      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ available: mockAvailableScripts })
      });

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(global.fetch).toHaveBeenCalledWith('/api/v1/scripts/available');
      });
    });

    it('should populate script path dropdown with fetched scripts', async () => {
      const user = userEvent.setup();
      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ available: mockAvailableScripts })
      });

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByText(/SCRIPT PATH/i)).toBeInTheDocument();
      });

      // Open dropdown to see options
      const dropdownToggle = document.querySelector('.custom-dropdown-toggle');
      await user.click(dropdownToggle);

      // Check that script options are rendered
      expect(screen.getByText('maintenance/backup.sh')).toBeInTheDocument();
      expect(screen.getByText('deployment/deploy-app')).toBeInTheDocument();
      expect(screen.getByText('monitoring/check-health.py')).toBeInTheDocument();
    });

    it('should show message when no scripts available', async () => {
      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ available: [] })
      });

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByText(/no unregistered scripts found/i)).toBeInTheDocument();
      });
    });
  });

  describe('Script Type Detection', () => {
    it('should auto-detect shell script type', async () => {
      const user = userEvent.setup();
      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ available: mockAvailableScripts })
      });

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByText(/SCRIPT PATH/i)).toBeInTheDocument();
      });

      await selectFromDropdown(user, /SCRIPT PATH/i, 'maintenance/backup.sh');

      await waitFor(() => {
        expect(screen.getByText('Shell')).toBeInTheDocument();
      });
    });

    it('should auto-detect go_binary script type', async () => {
      const user = userEvent.setup();
      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ available: mockAvailableScripts })
      });

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByText(/SCRIPT PATH/i)).toBeInTheDocument();
      });

      await selectFromDropdown(user, /SCRIPT PATH/i, 'deployment/deploy-app');

      await waitFor(() => {
        expect(screen.getByText('Go')).toBeInTheDocument();
      });
    });

    it('should auto-detect python script type', async () => {
      const user = userEvent.setup();
      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ available: mockAvailableScripts })
      });

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByText(/SCRIPT PATH/i)).toBeInTheDocument();
      });

      await selectFromDropdown(user, /SCRIPT PATH/i, 'monitoring/check-health.py');

      await waitFor(() => {
        expect(screen.getByText('Python')).toBeInTheDocument();
      });
    });
  });

  describe('Form Validation', () => {
    beforeEach(() => {
      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ available: mockAvailableScripts })
      });
    });

    it('should require script name', async () => {
      const user = userEvent.setup();

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /register/i })).toBeInTheDocument();
      });

      const registerButton = screen.getByRole('button', { name: /register/i });
      await user.click(registerButton);

      expect(screen.getByText(/script name is required/i)).toBeInTheDocument();
      expect(mockOnSave).not.toHaveBeenCalled();
    });

    it('should require script path', async () => {
      const user = userEvent.setup();

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByLabelText(/SCRIPT NAME/i)).toBeInTheDocument();
      });

      const nameInput = screen.getByLabelText(/SCRIPT NAME/i);
      await user.type(nameInput, 'Test Script');

      const registerButton = screen.getByRole('button', { name: /register/i });
      await user.click(registerButton);

      expect(screen.getByText(/script path is required/i)).toBeInTheDocument();
      expect(mockOnSave).not.toHaveBeenCalled();
    });

    it('should require category', async () => {
      const user = userEvent.setup();

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByLabelText(/SCRIPT NAME/i)).toBeInTheDocument();
      });

      const nameInput = screen.getByLabelText(/SCRIPT NAME/i);
      await user.type(nameInput, 'Test Script');

      await selectFromDropdown(user, /SCRIPT PATH/i, 'maintenance/backup.sh');

      const registerButton = screen.getByRole('button', { name: /register/i });
      await user.click(registerButton);

      expect(screen.getByText(/category is required/i)).toBeInTheDocument();
      expect(mockOnSave).not.toHaveBeenCalled();
    });

    it('should validate timeout is a positive number', async () => {
      const user = userEvent.setup();

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByLabelText(/TIMEOUT SECONDS/i)).toBeInTheDocument();
      });

      const timeoutInput = screen.getByLabelText(/TIMEOUT SECONDS/i);
      await user.clear(timeoutInput);
      await user.type(timeoutInput, '-5');

      const nameInput = screen.getByLabelText(/SCRIPT NAME/i);
      await user.type(nameInput, 'Test Script');

      await selectFromDropdown(user, /SCRIPT PATH/i, 'maintenance/backup.sh');

      const categoryInput = screen.getByLabelText(/CATEGORY/i);
      await user.type(categoryInput, 'maintenance');

      const registerButton = screen.getByRole('button', { name: /register/i });
      await user.click(registerButton);

      expect(screen.getByText(/timeout must be a positive number/i)).toBeInTheDocument();
      expect(mockOnSave).not.toHaveBeenCalled();
    });
  });

  describe('Category Suggestions', () => {
    beforeEach(() => {
      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ available: mockAvailableScripts })
      });
    });

    it('should provide category suggestions via datalist', async () => {
      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByLabelText(/CATEGORY/i)).toBeInTheDocument();
      });

      const categoryInput = screen.getByLabelText(/CATEGORY/i);
      expect(categoryInput).toHaveAttribute('list');

      const datalist = document.getElementById(categoryInput.getAttribute('list'));
      expect(datalist).toBeInTheDocument();

      const options = datalist.querySelectorAll('option');
      const values = Array.from(options).map(opt => opt.value);

      expect(values).toContain('maintenance');
      expect(values).toContain('deployment');
      expect(values).toContain('backup');
      expect(values).toContain('monitoring');
      expect(values).toContain('utility');
    });
  });

  describe('Advanced Section', () => {
    beforeEach(() => {
      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ available: mockAvailableScripts })
      });
    });

    it('should show advanced section toggle', async () => {
      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByText(/advanced settings/i)).toBeInTheDocument();
      });
    });

    it('should toggle advanced section visibility', async () => {
      const user = userEvent.setup();

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByText(/advanced settings/i)).toBeInTheDocument();
      });

      expect(screen.queryByLabelText(/ENVIRONMENT VARIABLES/i)).not.toBeInTheDocument();

      const advancedToggle = screen.getByText(/advanced settings/i);
      await user.click(advancedToggle);

      expect(screen.getByText(/ENVIRONMENT VARIABLES/i)).toBeInTheDocument();
    });

    it('should allow adding environment variables', async () => {
      const user = userEvent.setup();

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByText(/advanced settings/i)).toBeInTheDocument();
      });

      const advancedToggle = screen.getByText(/advanced settings/i);
      await user.click(advancedToggle);

      const addEnvButton = screen.getByText(/add variable/i);
      await user.click(addEnvButton);

      const keyInputs = screen.getAllByPlaceholderText(/key/i);
      const valueInputs = screen.getAllByPlaceholderText(/value/i);

      expect(keyInputs.length).toBeGreaterThan(0);
      expect(valueInputs.length).toBeGreaterThan(0);
    });

    it('should allow removing environment variables', async () => {
      const user = userEvent.setup();

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByText(/advanced settings/i)).toBeInTheDocument();
      });

      const advancedToggle = screen.getByText(/advanced settings/i);
      await user.click(advancedToggle);

      const addEnvButton = screen.getByText(/add variable/i);
      await user.click(addEnvButton);

      const keyInputsBefore = screen.getAllByPlaceholderText(/key/i);
      const initialCount = keyInputsBefore.length;

      const removeButtons = screen.getAllByLabelText(/remove variable/i);
      await user.click(removeButtons[0]);

      const keyInputsAfter = screen.queryAllByPlaceholderText(/key/i);
      expect(keyInputsAfter.length).toBe(initialCount - 1);
    });
  });

  describe('Form Submission', () => {
    beforeEach(() => {
      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ available: mockAvailableScripts })
      });
    });

    it('should submit valid form data', async () => {
      const user = userEvent.setup();

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByLabelText(/SCRIPT NAME/i)).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText(/SCRIPT NAME/i), 'Backup Script');
      await selectFromDropdown(user, /SCRIPT PATH/i, 'maintenance/backup.sh');
      await user.type(screen.getByLabelText(/CATEGORY/i), 'maintenance');
      await user.type(screen.getByLabelText(/DESCRIPTION/i), 'Daily backup script');

      const registerButton = screen.getByRole('button', { name: /register/i });
      await user.click(registerButton);

      expect(mockOnSave).toHaveBeenCalledWith(
        expect.objectContaining({
          name: 'Backup Script',
          scriptPath: 'maintenance/backup.sh',
          category: 'maintenance',
          description: 'Daily backup script',
          scriptType: 'shell',
          timeoutSeconds: 300
        })
      );
      expect(mockOnClose).toHaveBeenCalled();
    });

    it('should include environment variables in submission', async () => {
      const user = userEvent.setup();

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByLabelText(/SCRIPT NAME/i)).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText(/SCRIPT NAME/i), 'Deploy Script');
      await selectFromDropdown(user, /SCRIPT PATH/i, 'deployment/deploy-app');
      await user.type(screen.getByLabelText(/CATEGORY/i), 'deployment');

      const advancedToggle = screen.getByText(/advanced settings/i);
      await user.click(advancedToggle);

      const addEnvButton = screen.getByText(/add variable/i);
      await user.click(addEnvButton);

      const keyInputs = screen.getAllByPlaceholderText(/key/i);
      const valueInputs = screen.getAllByPlaceholderText(/value/i);

      await user.type(keyInputs[0], 'ENV_VAR');
      await user.type(valueInputs[0], 'production');

      const registerButton = screen.getByRole('button', { name: /register/i });
      await user.click(registerButton);

      expect(mockOnSave).toHaveBeenCalledWith(
        expect.objectContaining({
          environment: { ENV_VAR: 'production' }
        })
      );
    });

    it('should include custom timeout in submission', async () => {
      const user = userEvent.setup();

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByLabelText(/SCRIPT NAME/i)).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText(/SCRIPT NAME/i), 'Long Script');
      await selectFromDropdown(user, /SCRIPT PATH/i, 'maintenance/backup.sh');
      await user.type(screen.getByLabelText(/CATEGORY/i), 'maintenance');

      const timeoutInput = screen.getByLabelText(/TIMEOUT SECONDS/i);
      await user.clear(timeoutInput);
      await user.type(timeoutInput, '600');

      const registerButton = screen.getByRole('button', { name: /register/i });
      await user.click(registerButton);

      expect(mockOnSave).toHaveBeenCalledWith(
        expect.objectContaining({
          timeoutSeconds: 600
        })
      );
    });
  });

  describe('Modal Actions', () => {
    beforeEach(() => {
      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ available: mockAvailableScripts })
      });
    });

    it('should call onClose when cancel button is clicked', async () => {
      const user = userEvent.setup();

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByText('CANCEL')).toBeInTheDocument();
      });

      const cancelButton = screen.getByText('CANCEL');
      await user.click(cancelButton);

      expect(mockOnClose).toHaveBeenCalled();
      expect(mockOnSave).not.toHaveBeenCalled();
    });

    it('should disable register button while loading', async () => {
      global.fetch.mockImplementation(() => new Promise(() => {}));

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const registerButton = screen.getByRole('button', { name: /register/i });
      expect(registerButton).toBeDisabled();
    });
  });

  describe('Error Handling', () => {
    it('should handle API errors gracefully', async () => {
      global.fetch.mockRejectedValueOnce(new Error('API Error'));

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByText(/failed to load available scripts/i)).toBeInTheDocument();
      });
    });

    it('should allow retry after fetch error', async () => {
      const user = userEvent.setup();
      global.fetch
        .mockRejectedValueOnce(new Error('API Error'))
        .mockResolvedValueOnce({
          ok: true,
          json: async () => ({ available: mockAvailableScripts })
        });

      render(
        <RegisterScriptModal
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      await waitFor(() => {
        expect(screen.getByText(/failed to load available scripts/i)).toBeInTheDocument();
      });

      const retryButton = screen.getByText(/try again/i);
      await user.click(retryButton);

      await waitFor(() => {
        expect(screen.getByLabelText(/SCRIPT NAME/i)).toBeInTheDocument();
      });
    });
  });
});
