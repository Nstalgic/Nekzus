/**
 * WebhookTesterModal Component Tests
 *
 * Test coverage for webhook testing modal
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { WebhookTesterModal } from './WebhookTesterModal';
import { NotificationProvider } from '../../contexts/NotificationContext';
import { SettingsProvider } from '../../contexts/SettingsContext';

// Mock the API modules
vi.mock('../../services/api', () => ({
  devicesAPI: {
    list: vi.fn().mockResolvedValue([]),
  },
  webhooksAPI: {
    sendActivity: vi.fn().mockResolvedValue({ success: true }),
    sendNotify: vi.fn().mockResolvedValue({ success: true }),
  },
}));

import { devicesAPI, webhooksAPI } from '../../services/api';

const renderWithProvider = (ui) => {
  return render(
    <SettingsProvider>
      <NotificationProvider>
        {ui}
      </NotificationProvider>
    </SettingsProvider>
  );
};

describe('WebhookTesterModal', () => {
  const defaultProps = {
    isOpen: true,
    onClose: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    devicesAPI.list.mockResolvedValue([]);
  });

  describe('Rendering', () => {
    it('should render when open', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      expect(screen.getByText('WEBHOOK TESTER')).toBeInTheDocument();
    });

    it('should not render when closed', () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} isOpen={false} />);
      expect(screen.queryByText('WEBHOOK TESTER')).not.toBeInTheDocument();
    });

    it('should render both webhook type tabs', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      expect(screen.getByText('ACTIVITY WEBHOOK')).toBeInTheDocument();
      expect(screen.getByText('NOTIFY WEBHOOK')).toBeInTheDocument();
    });

    it('should render message input field', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      expect(screen.getByPlaceholderText('Test notification message')).toBeInTheDocument();
    });

    it('should render style dropdown', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      expect(screen.getByText('STYLE')).toBeInTheDocument();
    });

    it('should render send button', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      expect(screen.getByText('SEND WEBHOOK')).toBeInTheDocument();
    });

    it('should render device targeting section', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      expect(screen.getByText('TARGET DEVICES')).toBeInTheDocument();
    });

    it('should render payload preview section', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      expect(screen.getByText('PAYLOAD PREVIEW')).toBeInTheDocument();
    });
  });

  describe('User Interactions', () => {
    it('should allow typing in message field', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      const input = screen.getByPlaceholderText('Test notification message');
      fireEvent.change(input, { target: { value: 'Test message' } });
      expect(input.value).toBe('Test message');
    });

    it('should show activity tab as default', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      const activityTab = screen.getByText('ACTIVITY WEBHOOK');
      // CSS modules add unique suffixes to class names
      expect(activityTab.className).toMatch(/active/);
    });

    it('should allow switching between tabs', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      const notifyTab = screen.getByText('NOTIFY WEBHOOK');
      fireEvent.click(notifyTab);
      // CSS modules add unique suffixes to class names
      expect(notifyTab.className).toMatch(/active/);
    });

    it('should show notify form when notify tab is clicked', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      const notifyTab = screen.getByText('NOTIFY WEBHOOK');
      fireEvent.click(notifyTab);
      expect(screen.getByText('NOTIFICATION TYPE')).toBeInTheDocument();
      expect(screen.getByText('PAYLOAD DATA (JSON)')).toBeInTheDocument();
    });

    it('should close on X button click', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      const closeButton = screen.getByLabelText('Close modal');
      fireEvent.click(closeButton);
      expect(defaultProps.onClose).toHaveBeenCalledTimes(1);
    });

    it('should close on cancel button click', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      const cancelButton = screen.getByText('CANCEL');
      fireEvent.click(cancelButton);
      expect(defaultProps.onClose).toHaveBeenCalledTimes(1);
    });

    it('should have broadcast selected by default', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      const broadcastRadio = screen.getByLabelText(/broadcast to all devices/i);
      expect(broadcastRadio).toBeChecked();
    });

    it('should allow selecting specific devices mode', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);
      const specificRadio = screen.getByLabelText(/target specific devices/i);
      fireEvent.click(specificRadio);
      expect(specificRadio).toBeChecked();
    });
  });

  describe('Webhook Sending', () => {
    it('should send activity webhook when form is submitted', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);

      const input = screen.getByPlaceholderText('Test notification message');
      fireEvent.change(input, { target: { value: 'Test webhook' } });

      const sendButton = screen.getByText('SEND WEBHOOK');
      fireEvent.click(sendButton);

      await waitFor(() => {
        expect(webhooksAPI.sendActivity).toHaveBeenCalledWith(
          expect.objectContaining({
            message: 'Test webhook',
          })
        );
      });
    });

    it('should send notify webhook when notify tab is selected', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);

      // Switch to notify tab
      const notifyTab = screen.getByText('NOTIFY WEBHOOK');
      fireEvent.click(notifyTab);

      const sendButton = screen.getByText('SEND WEBHOOK');
      fireEvent.click(sendButton);

      await waitFor(() => {
        expect(webhooksAPI.sendNotify).toHaveBeenCalled();
      });
    });

    it('should disable send button while sending', async () => {
      webhooksAPI.sendActivity.mockImplementation(() => new Promise(resolve => setTimeout(resolve, 100)));

      renderWithProvider(<WebhookTesterModal {...defaultProps} />);

      const input = screen.getByPlaceholderText('Test notification message');
      fireEvent.change(input, { target: { value: 'Test' } });

      const sendButton = screen.getByText('SEND WEBHOOK');
      fireEvent.click(sendButton);

      expect(sendButton).toBeDisabled();
    });

    it('should prevent sending empty message', () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);

      const sendButton = screen.getByText('SEND WEBHOOK');
      expect(sendButton).toBeDisabled();
    });
  });

  describe('Device Loading', () => {
    it('should load devices when modal opens', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);

      await waitFor(() => {
        expect(devicesAPI.list).toHaveBeenCalled();
      });
    });

    it('should show empty state when no devices are available', async () => {
      devicesAPI.list.mockResolvedValue([]);
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);

      // Switch to specific devices mode
      const specificRadio = screen.getByLabelText(/target specific devices/i);
      fireEvent.click(specificRadio);

      await waitFor(() => {
        expect(screen.getByText(/no paired devices found/i)).toBeInTheDocument();
      });
    });

    it('should show device list when devices are available', async () => {
      devicesAPI.list.mockResolvedValue([
        { id: 'device-1', name: 'Test Device 1' },
        { id: 'device-2', name: 'Test Device 2' },
      ]);

      renderWithProvider(<WebhookTesterModal {...defaultProps} />);

      // Switch to specific devices mode
      const specificRadio = screen.getByLabelText(/target specific devices/i);
      fireEvent.click(specificRadio);

      await waitFor(() => {
        expect(screen.getByText('Test Device 1')).toBeInTheDocument();
        expect(screen.getByText('Test Device 2')).toBeInTheDocument();
      });
    });
  });

  describe('Payload Preview', () => {
    it('should show payload preview with message', async () => {
      renderWithProvider(<WebhookTesterModal {...defaultProps} />);

      const input = screen.getByPlaceholderText('Test notification message');
      fireEvent.change(input, { target: { value: 'Preview test' } });

      await waitFor(() => {
        const preview = screen.getByText(/preview test/i);
        expect(preview).toBeInTheDocument();
      });
    });
  });
});
