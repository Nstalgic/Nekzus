import { useNotification } from '../../contexts/NotificationContext';
import Toast from './Toast';

export default function ToastContainer() {
  const { notifications, markAsToasted } = useNotification();

  // Only show notifications that haven't been toasted yet and aren't dismissed
  const activeToasts = notifications.filter((n) => !n.toasted && !n.dismissed);

  // Show only the 3 most recent toasts
  const visibleToasts = activeToasts.slice(0, 3);

  return (
    <div className="toast-container">
      {visibleToasts.map((notification) => (
        <Toast
          key={notification.id}
          notification={notification}
          onDismiss={markAsToasted}
        />
      ))}
    </div>
  );
}
