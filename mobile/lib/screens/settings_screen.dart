import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../providers/theme_provider.dart';
import '../services/api_service.dart';
import '../widgets/admin_key_dialog.dart';

class SettingsScreen extends StatefulWidget {
  final ApiService? apiService;

  const SettingsScreen({super.key, this.apiService});

  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  late final ApiService _apiService;
  final TextEditingController _serverUrlController = TextEditingController();
  final _formKey = GlobalKey<FormState>();
  bool _isLoading = true;
  bool _isTesting = false;
  bool? _connectionSuccess;
  bool _hasAdminKey = false;
  bool? _authEnabled;

  @override
  void initState() {
    super.initState();
    _apiService = widget.apiService ?? ApiService();
    _loadSettings();
  }

  Future<void> _loadSettings() async {
    final serverUrl = await _apiService.getServerUrl();
    _serverUrlController.text = serverUrl;

    // Check auth status
    final hasKey = await _apiService.authService.hasAdminKey();
    final authEnabled = await _apiService.authService.checkAuthEnabled(
      serverUrl,
    );

    setState(() {
      _hasAdminKey = hasKey;
      _authEnabled = authEnabled;
      _isLoading = false;
    });
  }

  String? _validateUrl(String? value) {
    if (value == null || value.trim().isEmpty) {
      return 'Server URL cannot be empty';
    }

    final url = value.trim();
    try {
      final uri = Uri.parse(url);
      if (!uri.hasScheme || (uri.scheme != 'http' && uri.scheme != 'https')) {
        return 'URL must start with http:// or https://';
      }
      if (uri.host.isEmpty) {
        return 'Please enter a valid server address';
      }
    } catch (e) {
      return 'Please enter a valid URL';
    }

    return null;
  }

  Future<void> _testConnection() async {
    final url = _serverUrlController.text.trim();
    final validationError = _validateUrl(url);
    if (validationError != null) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(validationError),
          backgroundColor: Theme.of(context).colorScheme.error,
        ),
      );
      return;
    }

    setState(() {
      _isTesting = true;
      _connectionSuccess = null;
    });

    // Temporarily set the URL to test
    final originalUrl = await _apiService.getServerUrl();
    await _apiService.setServerUrl(url);

    try {
      final success = await _apiService.testConnection();
      if (mounted) {
        setState(() {
          _connectionSuccess = success;
          _isTesting = false;
        });

        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              success
                  ? 'Connection successful!'
                  : 'Could not connect to server',
            ),
            backgroundColor: success
                ? Colors.green
                : Theme.of(context).colorScheme.error,
            behavior: SnackBarBehavior.floating,
          ),
        );
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _connectionSuccess = false;
          _isTesting = false;
        });
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Connection failed: $e'),
            backgroundColor: Theme.of(context).colorScheme.error,
            behavior: SnackBarBehavior.floating,
          ),
        );
      }
    }

    // Restore original URL if test failed
    if (_connectionSuccess != true) {
      await _apiService.setServerUrl(originalUrl);
    }
  }

  Future<void> _saveSettings() async {
    if (!_formKey.currentState!.validate()) {
      return;
    }

    final url = _serverUrlController.text.trim();
    await _apiService.setServerUrl(url);

    if (!mounted) return;

    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(
        content: Text('Settings saved!'),
        backgroundColor: Colors.green,
        behavior: SnackBarBehavior.floating,
      ),
    );

    // Clear connection status after saving and refresh auth status
    final authEnabled = await _apiService.authService.checkAuthEnabled(url);
    setState(() {
      _connectionSuccess = null;
      _authEnabled = authEnabled;
    });
  }

  Future<void> _showAdminKeyDialog() async {
    final success = await AdminKeyDialog.show(context, _apiService);
    if (success && mounted) {
      setState(() => _hasAdminKey = true);
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Admin access granted!'),
          backgroundColor: Colors.green,
          behavior: SnackBarBehavior.floating,
        ),
      );
    }
  }

  Future<void> _clearAdminKey() async {
    final confirm = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Clear Admin Key?'),
        content: const Text(
          'You will need to re-enter the admin key to modify the collection.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Clear'),
          ),
        ],
      ),
    );

    if (confirm == true) {
      await _apiService.authService.clearAdminKey();
      if (mounted) {
        setState(() => _hasAdminKey = false);
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('Admin key cleared'),
            behavior: SnackBarBehavior.floating,
          ),
        );
      }
    }
  }

  @override
  void dispose() {
    _serverUrlController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final colorScheme = theme.colorScheme;

    return Scaffold(
      appBar: AppBar(title: const Text('Settings')),
      body: _isLoading
          ? const Center(child: CircularProgressIndicator())
          : Form(
              key: _formKey,
              child: ListView(
                padding: const EdgeInsets.all(16),
                children: [
                  // Theme Section
                  Text(
                    'Appearance',
                    style: theme.textTheme.titleMedium?.copyWith(
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    'Choose how the app looks.',
                    style: theme.textTheme.bodyMedium?.copyWith(
                      color: colorScheme.onSurfaceVariant,
                    ),
                  ),
                  const SizedBox(height: 16),
                  Consumer<ThemeProvider>(
                    builder: (context, themeProvider, child) {
                      return SegmentedButton<ThemeMode>(
                        segments: const [
                          ButtonSegment(
                            value: ThemeMode.system,
                            label: Text('System'),
                            icon: Icon(Icons.settings_suggest),
                          ),
                          ButtonSegment(
                            value: ThemeMode.light,
                            label: Text('Light'),
                            icon: Icon(Icons.light_mode),
                          ),
                          ButtonSegment(
                            value: ThemeMode.dark,
                            label: Text('Dark'),
                            icon: Icon(Icons.dark_mode),
                          ),
                        ],
                        selected: {themeProvider.themeMode},
                        onSelectionChanged: (selection) {
                          themeProvider.setThemeMode(selection.first);
                        },
                      );
                    },
                  ),
                  const SizedBox(height: 32),
                  const Divider(),
                  const SizedBox(height: 16),
                  // Server Configuration Section
                  Text(
                    'Server Configuration',
                    style: theme.textTheme.titleMedium?.copyWith(
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    'Enter the URL of your TCG Tracker backend server.',
                    style: theme.textTheme.bodyMedium?.copyWith(
                      color: colorScheme.onSurfaceVariant,
                    ),
                  ),
                  const SizedBox(height: 16),
                  TextFormField(
                    controller: _serverUrlController,
                    decoration: InputDecoration(
                      labelText: 'Server URL',
                      hintText: 'http://192.168.1.100:8080',
                      border: const OutlineInputBorder(),
                      prefixIcon: const Icon(Icons.link),
                      suffixIcon: _connectionSuccess != null
                          ? Icon(
                              _connectionSuccess!
                                  ? Icons.check_circle
                                  : Icons.error,
                              color: _connectionSuccess!
                                  ? Colors.green
                                  : colorScheme.error,
                            )
                          : null,
                    ),
                    keyboardType: TextInputType.url,
                    validator: _validateUrl,
                    onChanged: (_) {
                      // Clear connection status when URL changes
                      if (_connectionSuccess != null) {
                        setState(() => _connectionSuccess = null);
                      }
                    },
                  ),
                  const SizedBox(height: 16),
                  Row(
                    children: [
                      Expanded(
                        child: OutlinedButton.icon(
                          onPressed: _isTesting ? null : _testConnection,
                          icon: _isTesting
                              ? const SizedBox(
                                  width: 18,
                                  height: 18,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                  ),
                                )
                              : const Icon(Icons.wifi_find),
                          label: Text(
                            _isTesting ? 'Testing...' : 'Test Connection',
                          ),
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: FilledButton.icon(
                          onPressed: _saveSettings,
                          icon: const Icon(Icons.save),
                          label: const Text('Save'),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 32),
                  const Divider(),
                  const SizedBox(height: 16),
                  // Admin Key Section
                  Text(
                    'Admin Access',
                    style: theme.textTheme.titleMedium?.copyWith(
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    _authEnabled == true
                        ? 'An admin key is required to modify the collection on this server.'
                        : 'Authentication is not enabled on this server.',
                    style: theme.textTheme.bodyMedium?.copyWith(
                      color: colorScheme.onSurfaceVariant,
                    ),
                  ),
                  const SizedBox(height: 16),
                  if (_authEnabled == true) ...[
                    Card(
                      child: ListTile(
                        leading: Icon(
                          _hasAdminKey ? Icons.lock_open : Icons.lock,
                          color: _hasAdminKey
                              ? Colors.green
                              : colorScheme.primary,
                        ),
                        title: Text(
                          _hasAdminKey
                              ? 'Admin key configured'
                              : 'No admin key',
                        ),
                        subtitle: Text(
                          _hasAdminKey
                              ? 'You have admin access to modify the collection'
                              : 'Tap to enter your admin key',
                        ),
                        trailing: _hasAdminKey
                            ? TextButton(
                                onPressed: _clearAdminKey,
                                child: const Text('Clear'),
                              )
                            : const Icon(Icons.chevron_right),
                        onTap: _hasAdminKey ? null : _showAdminKeyDialog,
                      ),
                    ),
                  ] else if (_authEnabled == false) ...[
                    Card(
                      child: ListTile(
                        leading: Icon(Icons.lock_open, color: Colors.green),
                        title: const Text('No authentication required'),
                        subtitle: const Text(
                          'All collection operations are allowed',
                        ),
                      ),
                    ),
                  ],
                  const SizedBox(height: 32),
                  const Divider(),
                  const SizedBox(height: 16),
                  Card(
                    child: ListTile(
                      leading: Icon(
                        Icons.info_outline,
                        color: colorScheme.primary,
                      ),
                      title: const Text('TCG Tracker Mobile'),
                      subtitle: const Text('Version 1.0.0'),
                    ),
                  ),
                  const SizedBox(height: 8),
                  Card(
                    child: ListTile(
                      leading: Icon(
                        Icons.help_outline,
                        color: colorScheme.primary,
                      ),
                      title: const Text('Connection Tips'),
                      subtitle: const Text(
                        'Make sure your phone and server are on the same network. '
                        'Use your computer\'s local IP address (e.g., 192.168.x.x), not localhost.',
                      ),
                    ),
                  ),
                ],
              ),
            ),
    );
  }
}
