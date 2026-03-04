import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Modal } from './Modal';

/**
 * Modal Component Test Suite
 *
 * Tests the base Modal component including:
 * - Rendering and visibility
 * - ESC key handling
 * - Click-outside-to-close behavior
 * - Scroll lock
 * - Accessibility features
 */

describe('Modal', () => {
  describe('Rendering', () => {
    it('should not render when isOpen is false', () => {
      render(
        <Modal isOpen={false} onClose={() => {}}>
          <div>Modal Content</div>
        </Modal>
      );

      expect(screen.queryByText('Modal Content')).not.toBeInTheDocument();
    });

    it('should render when isOpen is true', () => {
      render(
        <Modal isOpen={true} onClose={() => {}}>
          <div>Modal Content</div>
        </Modal>
      );

      expect(screen.getByText('Modal Content')).toBeInTheDocument();
    });

    it('should render children content', () => {
      render(
        <Modal isOpen={true} onClose={() => {}}>
          <div>Test Title</div>
          <p>Test Description</p>
          <button>Test Button</button>
        </Modal>
      );

      expect(screen.getByText('Test Title')).toBeInTheDocument();
      expect(screen.getByText('Test Description')).toBeInTheDocument();
      expect(screen.getByText('Test Button')).toBeInTheDocument();
    });
  });

  describe('Size Variants', () => {
    it('should apply small size class', () => {
      const { container } = render(
        <Modal isOpen={true} onClose={() => {}} size="small">
          <div>Small Modal</div>
        </Modal>
      );

      // CSS modules add unique suffixes, so use partial class match
      const modalContent = container.querySelector('[class*="modalContent"]');
      expect(modalContent).toBeInTheDocument();
      expect(modalContent.className).toMatch(/modalSmall/);
    });

    it('should apply medium size class (default)', () => {
      const { container } = render(
        <Modal isOpen={true} onClose={() => {}}>
          <div>Medium Modal</div>
        </Modal>
      );

      const modalContent = container.querySelector('[class*="modalContent"]');
      expect(modalContent).toBeInTheDocument();
      expect(modalContent.className).toMatch(/modalMedium/);
    });

    it('should apply large size class', () => {
      const { container } = render(
        <Modal isOpen={true} onClose={() => {}} size="large">
          <div>Large Modal</div>
        </Modal>
      );

      const modalContent = container.querySelector('[class*="modalContent"]');
      expect(modalContent).toBeInTheDocument();
      expect(modalContent.className).toMatch(/modalLarge/);
    });
  });

  describe('ESC Key Handling', () => {
    it('should close modal when ESC is pressed (default behavior)', async () => {
      const user = userEvent.setup();
      const onClose = vi.fn();

      render(
        <Modal isOpen={true} onClose={onClose}>
          <div>Modal Content</div>
        </Modal>
      );

      await user.keyboard('{Escape}');

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('should not close modal when ESC is pressed if closeOnEscape is false', async () => {
      const user = userEvent.setup();
      const onClose = vi.fn();

      render(
        <Modal isOpen={true} onClose={onClose} closeOnEscape={false}>
          <div>Modal Content</div>
        </Modal>
      );

      await user.keyboard('{Escape}');

      expect(onClose).not.toHaveBeenCalled();
    });
  });

  describe('Click Outside to Close', () => {
    it('should close modal when clicking on overlay (default behavior)', async () => {
      const user = userEvent.setup();
      const onClose = vi.fn();

      const { container } = render(
        <Modal isOpen={true} onClose={onClose}>
          <div>Modal Content</div>
        </Modal>
      );

      const overlay = container.querySelector('[class*="modalOverlay"]');
      await user.click(overlay);

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('should not close modal when clicking on modal content', async () => {
      const user = userEvent.setup();
      const onClose = vi.fn();

      render(
        <Modal isOpen={true} onClose={onClose}>
          <div>Modal Content</div>
        </Modal>
      );

      const content = screen.getByText('Modal Content');
      await user.click(content);

      expect(onClose).not.toHaveBeenCalled();
    });

    it('should not close modal when clicking overlay if closeOnOverlay is false', async () => {
      const user = userEvent.setup();
      const onClose = vi.fn();

      const { container } = render(
        <Modal isOpen={true} onClose={onClose} closeOnOverlay={false}>
          <div>Modal Content</div>
        </Modal>
      );

      const overlay = container.querySelector('[class*="modalOverlay"]');
      await user.click(overlay);

      expect(onClose).not.toHaveBeenCalled();
    });
  });

  describe('Scroll Lock', () => {
    it('should lock body scroll when modal opens', () => {
      const { rerender } = render(
        <Modal isOpen={false} onClose={() => {}}>
          <div>Modal Content</div>
        </Modal>
      );

      // Initially, body should not have overflow hidden
      expect(document.body.style.overflow).toBe('');

      // Open modal
      rerender(
        <Modal isOpen={true} onClose={() => {}}>
          <div>Modal Content</div>
        </Modal>
      );

      expect(document.body.style.overflow).toBe('hidden');
    });

    it('should restore body scroll when modal closes', () => {
      const { rerender, unmount } = render(
        <Modal isOpen={true} onClose={() => {}}>
          <div>Modal Content</div>
        </Modal>
      );

      expect(document.body.style.overflow).toBe('hidden');

      // Close modal
      rerender(
        <Modal isOpen={false} onClose={() => {}}>
          <div>Modal Content</div>
        </Modal>
      );

      // Unmount to trigger cleanup
      unmount();

      expect(document.body.style.overflow).toBe('');
    });
  });

  describe('Accessibility', () => {
    it('should have role="dialog"', () => {
      const { container } = render(
        <Modal isOpen={true} onClose={() => {}}>
          <div>Modal Content</div>
        </Modal>
      );

      const modalOverlay = container.querySelector('[class*="modalOverlay"]');
      expect(modalOverlay).toHaveAttribute('role', 'dialog');
    });

    it('should have aria-modal="true"', () => {
      const { container } = render(
        <Modal isOpen={true} onClose={() => {}}>
          <div>Modal Content</div>
        </Modal>
      );

      const modalOverlay = container.querySelector('[class*="modalOverlay"]');
      expect(modalOverlay).toHaveAttribute('aria-modal', 'true');
    });
  });
});
