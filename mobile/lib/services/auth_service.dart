import 'dart:convert';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;

/// Service for managing admin key authentication
class AuthService {
  static const String _adminKeyStorageKey = 'admin_key';

  final FlutterSecureStorage _secureStorage;
  final http.Client _httpClient;

  // Cached values
  String? _cachedAdminKey;
  bool? _cachedAuthEnabled;

  AuthService({FlutterSecureStorage? secureStorage, http.Client? httpClient})
    : _secureStorage = secureStorage ?? const FlutterSecureStorage(),
      _httpClient = httpClient ?? http.Client();

  /// Get the stored admin key
  Future<String?> getAdminKey() async {
    _cachedAdminKey ??= await _secureStorage.read(key: _adminKeyStorageKey);
    return _cachedAdminKey;
  }

  /// Check if an admin key is stored
  Future<bool> hasAdminKey() async {
    final key = await getAdminKey();
    return key != null && key.isNotEmpty;
  }

  /// Store the admin key securely
  Future<void> setAdminKey(String key) async {
    await _secureStorage.write(key: _adminKeyStorageKey, value: key);
    _cachedAdminKey = key;
  }

  /// Clear the stored admin key
  Future<void> clearAdminKey() async {
    await _secureStorage.delete(key: _adminKeyStorageKey);
    _cachedAdminKey = null;
  }

  /// Check if authentication is enabled on the server
  Future<bool> checkAuthEnabled(String serverUrl) async {
    try {
      final uri = Uri.parse('$serverUrl/api/auth/status');
      final response = await _httpClient
          .get(uri)
          .timeout(
            const Duration(seconds: 10),
            onTimeout: () => throw Exception('Request timed out'),
          );

      if (response.statusCode == 200) {
        final data = json.decode(response.body);
        _cachedAuthEnabled = data['auth_enabled'] ?? false;
        return _cachedAuthEnabled!;
      }
      return false;
    } catch (e) {
      // If we can't check, assume auth might be enabled
      return true;
    }
  }

  /// Get cached auth enabled status (null if not checked yet)
  bool? get cachedAuthEnabled => _cachedAuthEnabled;

  /// Verify the admin key with the server
  /// Returns true if the key is valid
  Future<AuthVerifyResult> verifyKey(String serverUrl, String key) async {
    try {
      final uri = Uri.parse('$serverUrl/api/auth/verify');
      final response = await _httpClient
          .post(uri, headers: {'Authorization': 'Bearer $key'})
          .timeout(
            const Duration(seconds: 10),
            onTimeout: () => throw Exception('Request timed out'),
          );

      if (response.statusCode == 200) {
        final data = json.decode(response.body);
        if (data['valid'] == true) {
          return AuthVerifyResult.valid;
        }
      }

      if (response.statusCode == 401) {
        final data = json.decode(response.body);
        final code = data['code'] as String?;
        if (code == 'AUTH_INVALID_KEY') {
          return AuthVerifyResult.invalidKey;
        }
      }

      return AuthVerifyResult.error;
    } catch (e) {
      return AuthVerifyResult.error;
    }
  }

  /// Get authorization headers for API requests
  /// Returns empty map if no admin key is stored
  Future<Map<String, String>> getAuthHeaders() async {
    final key = await getAdminKey();
    if (key != null && key.isNotEmpty) {
      return {'Authorization': 'Bearer $key'};
    }
    return {};
  }
}

/// Result of admin key verification
enum AuthVerifyResult { valid, invalidKey, error }

/// Exception thrown when authentication is required but not provided
class AuthRequiredException implements Exception {
  final String message;
  AuthRequiredException([this.message = 'Authentication required']);

  @override
  String toString() => message;
}
