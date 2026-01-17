import 'dart:io';
import 'package:flutter/material.dart';
import 'package:camera/camera.dart';
import 'package:permission_handler/permission_handler.dart';
import '../models/card.dart';
import '../services/api_service.dart';
import '../services/camera_service.dart';
import '../services/ocr_service.dart';
import 'scan_result_screen.dart';

class CameraScreen extends StatefulWidget {
  final CameraService? cameraService;
  final OcrService? ocrService;
  final ApiService? apiService;

  const CameraScreen({
    super.key,
    this.cameraService,
    this.ocrService,
    this.apiService,
  });

  @override
  State<CameraScreen> createState() => _CameraScreenState();
}

class _CameraScreenState extends State<CameraScreen> {
  CameraController? _controller;
  List<CameraDescription>? _cameras;
  bool _isInitialized = false;
  bool _isProcessing = false;
  String _selectedGame = 'mtg';
  late final CameraService _cameraService;
  late final OcrService _ocrService;
  late final ApiService _apiService;
  bool? _serverOCRAvailable;

  @override
  void initState() {
    super.initState();
    _cameraService = widget.cameraService ?? CameraService();
    _ocrService = widget.ocrService ?? OcrService();
    _apiService = widget.apiService ?? ApiService();
    _initializeCamera();
    _checkServerOCR();
  }

  Future<void> _checkServerOCR() async {
    try {
      final available = await _apiService.isServerOCRAvailable();
      if (mounted) {
        setState(() => _serverOCRAvailable = available);
      }
    } catch (e) {
      // Server OCR check failed, assume unavailable
      if (mounted) {
        setState(() => _serverOCRAvailable = false);
      }
    }
  }

  Future<void> _initializeCamera() async {
    // Check camera permission first
    final status = await Permission.camera.status;
    if (!status.isGranted) {
      final result = await Permission.camera.request();
      if (!result.isGranted) {
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: const Text('Camera permission denied'),
              action: SnackBarAction(
                label: 'Settings',
                onPressed: () => openAppSettings(),
              ),
            ),
          );
        }
        return;
      }
    }

    _cameras = await _cameraService.getAvailableCameras();
    if (_cameras == null || _cameras!.isEmpty) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('No camera available')),
        );
      }
      return;
    }

    _controller = _cameraService.createController(
      _cameras!.first,
      resolutionPreset: ResolutionPreset.high,
      enableAudio: false,
    );

    try {
      await _controller!.initialize();
      if (mounted) {
        setState(() => _isInitialized = true);
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Failed to initialize camera: $e')),
        );
      }
    }
  }

  Future<void> _captureAndProcess() async {
    if (_controller == null || !_controller!.value.isInitialized || _isProcessing) {
      return;
    }

    setState(() => _isProcessing = true);

    try {
      final image = await _controller!.takePicture();
      final imageBytes = await File(image.path).readAsBytes();

      // Try client-side OCR first
      ScanResult? scanResult;
      bool usedServerOCR = false;

      try {
        final ocrResult = await _ocrService.processImage(image.path);

        if (ocrResult.textLines.isNotEmpty) {
          // Send full OCR text to server for parsing
          final fullText = ocrResult.textLines.join('\n');

          scanResult = await _apiService.identifyCard(
            fullText,
            _selectedGame,
            imageAnalysis: ocrResult.imageAnalysis,
          );
        }
      } on OcrException {
        // Client OCR failed, will try server OCR
      }

      // If client OCR failed or returned no cards, try server-side OCR
      if ((scanResult == null || scanResult.cards.isEmpty) && _serverOCRAvailable == true) {
        try {
          scanResult = await _apiService.identifyCardFromImage(
            imageBytes.toList(),
            _selectedGame,
          );
          usedServerOCR = true;
        } catch (e) {
          // Server OCR also failed
          if (scanResult == null) {
            rethrow;
          }
        }
      }

      // Clean up temp image
      await File(image.path).delete();

      if (!mounted) return;

      if (scanResult == null || scanResult.cards.isEmpty) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('No cards found')),
        );
        return;
      }

      // Navigate to results - best match should be first
      final result = scanResult;
      Navigator.push(
        context,
        MaterialPageRoute(
          builder: (context) => ScanResultScreen(
            cards: result.cards,
            searchQuery: usedServerOCR ? 'Scanned Card (Server OCR)' : 'Scanned Card',
            scanMetadata: result.metadata,
          ),
        ),
      );
    } catch (e) {
      if (mounted) {
        // Show user-friendly error message
        String message = 'An error occurred';
        final errorStr = e.toString();
        if (errorStr.contains('timed out')) {
          message = 'Request timed out. Check your connection.';
        } else if (errorStr.contains('SocketException') || errorStr.contains('Connection')) {
          message = 'Cannot connect to server. Check your network.';
        } else if (errorStr.contains('No text detected')) {
          message = 'No text detected in image. Try again with better lighting.';
        } else {
          message = 'Error: ${e.toString().replaceAll('Exception: ', '')}';
        }
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(message)),
        );
      }
    } finally {
      if (mounted) {
        setState(() => _isProcessing = false);
      }
    }
  }

  @override
  void dispose() {
    _controller?.dispose();
    _ocrService.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Scan Card'),
        backgroundColor: Theme.of(context).colorScheme.inversePrimary,
        actions: [
          IconButton(
            icon: const Icon(Icons.settings),
            onPressed: () => Navigator.pushNamed(context, '/settings'),
          ),
        ],
      ),
      body: Column(
        children: [
          // Game selector
          Padding(
            padding: const EdgeInsets.all(8.0),
            child: SegmentedButton<String>(
              segments: const [
                ButtonSegment(value: 'mtg', label: Text('MTG')),
                ButtonSegment(value: 'pokemon', label: Text('Pokemon')),
              ],
              selected: {_selectedGame},
              onSelectionChanged: (selection) {
                setState(() => _selectedGame = selection.first);
              },
            ),
          ),
          // Camera preview
          Expanded(
            child: Center(
              child: _isInitialized && _controller != null
                  ? ClipRRect(
                      borderRadius: BorderRadius.circular(12),
                      child: CameraPreview(_controller!),
                    )
                  : const CircularProgressIndicator(),
            ),
          ),
          // Capture button
          Padding(
            padding: const EdgeInsets.all(24.0),
            child: Center(
              child: FloatingActionButton.large(
                onPressed: _isProcessing ? null : _captureAndProcess,
                child: _isProcessing
                    ? const CircularProgressIndicator(color: Colors.white)
                    : const Icon(Icons.camera_alt),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
