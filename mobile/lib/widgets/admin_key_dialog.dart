import 'package:flutter/material.dart';
import '../services/auth_service.dart';
import '../services/api_service.dart';

/// Dialog for entering the admin key
class AdminKeyDialog extends StatefulWidget {
  final ApiService apiService;
  final VoidCallback? onSuccess;

  const AdminKeyDialog({super.key, required this.apiService, this.onSuccess});

  /// Show the admin key dialog
  static Future<bool> show(BuildContext context, ApiService apiService) async {
    final result = await showDialog<bool>(
      context: context,
      barrierDismissible: false,
      builder: (context) => AdminKeyDialog(apiService: apiService),
    );
    return result ?? false;
  }

  @override
  State<AdminKeyDialog> createState() => _AdminKeyDialogState();
}

class _AdminKeyDialogState extends State<AdminKeyDialog> {
  final TextEditingController _keyController = TextEditingController();
  final FocusNode _focusNode = FocusNode();
  bool _isVerifying = false;
  String? _errorMessage;
  bool _obscureText = true;

  @override
  void initState() {
    super.initState();
    // Auto-focus the text field
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _focusNode.requestFocus();
    });
  }

  @override
  void dispose() {
    _keyController.dispose();
    _focusNode.dispose();
    super.dispose();
  }

  Future<void> _verifyAndSave() async {
    final key = _keyController.text.trim();
    if (key.isEmpty) {
      setState(() => _errorMessage = 'Please enter an admin key');
      return;
    }

    setState(() {
      _isVerifying = true;
      _errorMessage = null;
    });

    // Capture navigator before async gap
    final navigator = Navigator.of(context);

    try {
      final serverUrl = await widget.apiService.getServerUrl();
      final result = await widget.apiService.authService.verifyKey(
        serverUrl,
        key,
      );

      if (!mounted) return;

      switch (result) {
        case AuthVerifyResult.valid:
          await widget.apiService.authService.setAdminKey(key);
          widget.onSuccess?.call();
          navigator.pop(true);
          break;
        case AuthVerifyResult.invalidKey:
          setState(() {
            _isVerifying = false;
            _errorMessage = 'Invalid admin key';
          });
          break;
        case AuthVerifyResult.error:
          setState(() {
            _isVerifying = false;
            _errorMessage = 'Failed to verify key. Check your connection.';
          });
          break;
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _isVerifying = false;
          _errorMessage = 'Error: ${e.toString()}';
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final colorScheme = theme.colorScheme;

    return AlertDialog(
      title: Row(
        children: [
          Icon(Icons.lock, color: colorScheme.primary),
          const SizedBox(width: 12),
          const Text('Admin Access'),
        ],
      ),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Enter the admin key to modify the collection.',
            style: theme.textTheme.bodyMedium?.copyWith(
              color: colorScheme.onSurfaceVariant,
            ),
          ),
          const SizedBox(height: 16),
          TextField(
            controller: _keyController,
            focusNode: _focusNode,
            obscureText: _obscureText,
            enabled: !_isVerifying,
            decoration: InputDecoration(
              labelText: 'Admin Key',
              border: const OutlineInputBorder(),
              prefixIcon: const Icon(Icons.key),
              suffixIcon: IconButton(
                icon: Icon(
                  _obscureText ? Icons.visibility : Icons.visibility_off,
                ),
                onPressed: () => setState(() => _obscureText = !_obscureText),
              ),
              errorText: _errorMessage,
            ),
            onSubmitted: (_) => _verifyAndSave(),
          ),
          const SizedBox(height: 8),
          Text(
            'The key will be stored securely on your device.',
            style: theme.textTheme.bodySmall?.copyWith(
              color: colorScheme.onSurfaceVariant,
            ),
          ),
        ],
      ),
      actions: [
        TextButton(
          onPressed: _isVerifying
              ? null
              : () => Navigator.of(context).pop(false),
          child: const Text('Cancel'),
        ),
        FilledButton(
          onPressed: _isVerifying ? null : _verifyAndSave,
          child: _isVerifying
              ? const SizedBox(
                  width: 20,
                  height: 20,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Text('Unlock'),
        ),
      ],
    );
  }
}
