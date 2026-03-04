/**
 * WebSocket Service
 *
 * Manages WebSocket connection to the Nekzus backend for real-time updates.
 * Handles authentication, reconnection, and event distribution.
 */

import { wsDebug } from '../utils/debug';

/**
 * WebSocket message types from backend
 */
export const WS_MSG_TYPES = {
  DISCOVERY: 'discovery',
  CONFIG_RELOAD: 'config_reload',
  CONFIG_WARNING: 'config_warning',
  DEVICE_PAIRED: 'device_paired',
  DEVICE_REVOKED: 'device_revoked',
  HEALTH_CHANGE: 'health_change',
  PORT_EXPOSURE: 'port_exposure_warning',
  WEBHOOK: 'webhook',
  HELLO: 'hello',
  PING: 'ping',
  PONG: 'pong',
  AUTH: 'auth',
  AUTH_SUCCESS: 'auth_success',
  AUTH_FAILED: 'auth_failed',
  APP_REGISTERED: 'app_registered',
  PROPOSAL_DISMISSED: 'proposal_dismissed',
  // Container logs streaming
  CONTAINER_LOGS_SUBSCRIBE: 'container.logs.subscribe',
  CONTAINER_LOGS_UNSUBSCRIBE: 'container.logs.unsubscribe',
  CONTAINER_LOGS: 'container.logs',
  CONTAINER_LOGS_STARTED: 'container.logs.started',
  CONTAINER_LOGS_ENDED: 'container.logs.ended',
  CONTAINER_LOGS_ERROR: 'container.logs.error',
  // Script execution events
  EXECUTION_STARTED: 'execution_started',
  EXECUTION_COMPLETED: 'execution_completed',
  EXECUTION_FAILED: 'execution_failed',
  // MQTT-style subscription messages
  SUBSCRIBE: 'subscribe',
  UNSUBSCRIBE: 'unsubscribe',
  SUBACK: 'suback',
  UNSUBACK: 'unsuback',
  // MQTT-style QoS messages
  ACK: 'ack',
  PUBREC: 'pubrec',
  PUBREL: 'pubrel',
  PUBCOMP: 'pubcomp',
  // Last Will and Testament
  SET_LAST_WILL: 'set_last_will',
  LWTACK: 'lwtack',
};

/**
 * QoS levels for message delivery
 */
export const QOS = {
  AT_MOST_ONCE: 0, // Fire and forget
  AT_LEAST_ONCE: 1, // Acknowledged delivery
  EXACTLY_ONCE: 2, // Exactly once delivery
};

/**
 * WebSocketService class
 *
 * Singleton service for managing WebSocket connection.
 * Usage:
 *   const ws = new WebSocketService();
 *   ws.on('discovery', (data) => console.log('New discovery', data));
 *   ws.connect();
 */
export class WebSocketService {
  constructor() {
    this.ws = null;
    this.listeners = new Map(); // Map<type, Set<callback>>
    this.reconnectTimer = null;
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = 10;
    this.reconnectDelay = 1000; // Start with 1 second
    this.maxReconnectDelay = 30000; // Max 30 seconds
    this.isConnected = false;
    this.isAuthenticated = false;
    this.token = null; // Optional JWT token for auth
    this.onConnectionChange = null; // Callback for connection state changes
  }

  /**
   * Get WebSocket URL based on current protocol
   * @returns {string} WebSocket URL
   */
  getWebSocketURL() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    return `${protocol}//${host}/api/v1/ws`;
  }

  /**
   * Connect to WebSocket server
   * @param {string} [token] - Optional JWT token for authentication
   */
  connect(token = null) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      return;
    }

    this.token = token;
    const url = this.getWebSocketURL();

    wsDebug.connect(url);

    try {
      this.ws = new WebSocket(url);

      this.ws.onopen = () => {
        wsDebug.open();
        this.isConnected = true;
        this.reconnectAttempts = 0;
        this.reconnectDelay = 1000;

        // Send auth message
        this.sendAuthMessage();

        // Notify connection state change
        if (this.onConnectionChange) {
          this.onConnectionChange({ connected: true, authenticated: false });
        }
      };

      this.ws.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);
          this.handleMessage(message);
        } catch (error) {
          wsDebug.error(error);
        }
      };

      this.ws.onerror = (error) => {
        wsDebug.error(error);
      };

      this.ws.onclose = (event) => {
        wsDebug.close(event.code, event.reason);
        this.isConnected = false;
        this.isAuthenticated = false;

        // Notify connection state change
        if (this.onConnectionChange) {
          this.onConnectionChange({ connected: false, authenticated: false });
        }

        // Attempt reconnection
        if (!event.wasClean) {
          this.scheduleReconnect();
        }
      };
    } catch (error) {
      wsDebug.error(error);
      this.scheduleReconnect();
    }
  }

  /**
   * Send authentication message to server
   */
  sendAuthMessage() {
    const authMessage = {
      type: WS_MSG_TYPES.AUTH,
      data: {
        token: this.token || '', // Send empty token for anonymous auth
      },
    };

    this.send(authMessage);
  }

  /**
   * Handle incoming WebSocket message
   * @param {Object} message - WebSocket message
   */
  handleMessage(message) {
    const { type, data, timestamp } = message;

    // Log received message (when enabled)
    wsDebug.receive(type, data);

    // Handle auth responses
    if (type === WS_MSG_TYPES.AUTH_SUCCESS) {
      this.isAuthenticated = true;

      // Notify connection state change
      if (this.onConnectionChange) {
        this.onConnectionChange({ connected: true, authenticated: true });
      }
    } else if (type === WS_MSG_TYPES.AUTH_FAILED) {
      this.isAuthenticated = false;
      this.disconnect();
      return;
    }

    // Handle ping messages (respond with pong)
    if (type === WS_MSG_TYPES.PING) {
      this.send({ type: WS_MSG_TYPES.PONG, data: {} });
      return;
    }

    // Emit message to registered listeners
    this.emit(type, data, timestamp);

    // Emit to wildcard listeners
    this.emit('*', { type, data, timestamp });
  }

  /**
   * Send message to WebSocket server
   * @param {Object} message - Message to send
   */
  send(message) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return false;
    }

    try {
      wsDebug.send(message.type, message.data);
      this.ws.send(JSON.stringify(message));
      return true;
    } catch (error) {
      wsDebug.error(error);
      return false;
    }
  }

  /**
   * Disconnect from WebSocket server
   */
  disconnect() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }

    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }

    this.isConnected = false;
    this.isAuthenticated = false;
  }

  /**
   * Schedule reconnection attempt
   */
  scheduleReconnect() {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      return;
    }

    this.reconnectAttempts++;

    // Exponential backoff with jitter
    const delay = Math.min(
      this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1),
      this.maxReconnectDelay
    );
    const jitter = Math.random() * 1000;
    const totalDelay = delay + jitter;

    wsDebug.reconnect(this.reconnectAttempts, this.maxReconnectAttempts, totalDelay);

    this.reconnectTimer = setTimeout(() => {
      this.connect(this.token);
    }, totalDelay);
  }

  /**
   * Register event listener
   * @param {string} type - Message type (or '*' for all messages)
   * @param {Function} callback - Callback function (data, timestamp) => void
   */
  on(type, callback) {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set());
    }
    this.listeners.get(type).add(callback);
  }

  /**
   * Unregister event listener
   * @param {string} type - Message type
   * @param {Function} callback - Callback function to remove
   */
  off(type, callback) {
    if (this.listeners.has(type)) {
      this.listeners.get(type).delete(callback);
    }
  }

  /**
   * Emit event to registered listeners
   * @param {string} type - Message type
   * @param {*} data - Message data
   * @param {string} timestamp - Message timestamp
   */
  emit(type, data, timestamp) {
    if (this.listeners.has(type)) {
      this.listeners.get(type).forEach((callback) => {
        try {
          callback(data, timestamp);
        } catch (error) {
          console.error(`WebSocket: Error in ${type} listener`, error);
        }
      });
    }
  }

  /**
   * Get connection status
   * @returns {Object} Status object
   */
  getStatus() {
    return {
      connected: this.isConnected,
      authenticated: this.isAuthenticated,
      reconnectAttempts: this.reconnectAttempts,
    };
  }

  /**
   * Subscribe to topics with optional QoS level
   * @param {string[]} topics - Array of topic patterns to subscribe to
   * @param {number} [qos=0] - Quality of Service level (0, 1, or 2)
   * @returns {boolean} True if message was sent
   */
  subscribe(topics, qos = 0) {
    if (!Array.isArray(topics) || topics.length === 0) {
      console.warn('WebSocket: subscribe requires an array of topics');
      return false;
    }

    return this.send({
      type: WS_MSG_TYPES.SUBSCRIBE,
      data: { topics, qos },
    });
  }

  /**
   * Unsubscribe from topics
   * @param {string[]} topics - Array of topic patterns to unsubscribe from
   * @returns {boolean} True if message was sent
   */
  unsubscribe(topics) {
    if (!Array.isArray(topics) || topics.length === 0) {
      console.warn('WebSocket: unsubscribe requires an array of topics');
      return false;
    }

    return this.send({
      type: WS_MSG_TYPES.UNSUBSCRIBE,
      data: { topics },
    });
  }

  /**
   * Set last will message to be published on unexpected disconnect
   * @param {string} topic - Topic to publish to
   * @param {*} message - Message payload
   * @param {number} [qos=0] - Quality of Service level
   * @returns {boolean} True if message was sent
   */
  setLastWill(topic, message, qos = 0) {
    if (!topic) {
      console.warn('WebSocket: setLastWill requires a topic');
      return false;
    }

    return this.send({
      type: WS_MSG_TYPES.SET_LAST_WILL,
      data: { topic, message, qos },
    });
  }

  /**
   * Acknowledge a QoS 1 message
   * @param {string} messageId - Message ID to acknowledge
   * @returns {boolean} True if message was sent
   */
  ack(messageId) {
    if (!messageId) {
      console.warn('WebSocket: ack requires a messageId');
      return false;
    }

    return this.send({
      type: WS_MSG_TYPES.ACK,
      messageId,
      data: { messageId },
    });
  }

  /**
   * Publish a message to a topic (for future use with pub/sub)
   * @param {string} topic - Topic to publish to
   * @param {*} data - Message payload
   * @param {Object} [options] - Publish options
   * @param {number} [options.qos=0] - Quality of Service level
   * @param {boolean} [options.retain=false] - Retain message for new subscribers
   * @param {number} [options.ttl] - Time to live in milliseconds
   * @returns {boolean} True if message was sent
   */
  publish(topic, data, options = {}) {
    const { qos = 0, retain = false, ttl } = options;

    const message = {
      type: topic,
      topic,
      data,
      qos,
      retain,
    };

    if (ttl) {
      message.expiresAt = new Date(Date.now() + ttl).toISOString();
    }

    return this.send(message);
  }
}

// Export singleton instance
export const websocketService = new WebSocketService();
