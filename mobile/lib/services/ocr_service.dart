import 'dart:io';
import 'package:google_mlkit_text_recognition/google_mlkit_text_recognition.dart';
import 'image_analysis_service.dart';

/// Combined result from OCR and image analysis
class OcrResult {
  final List<String> textLines;
  final ImageAnalysisResult? imageAnalysis;

  OcrResult({
    required this.textLines,
    this.imageAnalysis,
  });
}

/// Abstraction layer for OCR operations to enable testing
class OcrService {
  final TextRecognizer _recognizer;
  final ImageAnalysisService _imageAnalysisService;

  OcrService({
    TextRecognizer? recognizer,
    ImageAnalysisService? imageAnalysisService,
  })  : _recognizer = recognizer ?? TextRecognizer(),
        _imageAnalysisService = imageAnalysisService ?? ImageAnalysisService();

  /// Extract text lines from an image file
  /// Throws [OcrException] if the image cannot be processed
  Future<List<String>> extractTextFromImage(String imagePath) async {
    // Validate file exists
    final file = File(imagePath);
    if (!await file.exists()) {
      throw OcrException('Image file not found: $imagePath');
    }

    try {
      final inputImage = InputImage.fromFilePath(imagePath);
      final recognizedText = await _recognizer.processImage(inputImage);

      return recognizedText.blocks
          .expand((block) => block.lines)
          .map((line) => line.text)
          .toList();
    } catch (e) {
      if (e is OcrException) rethrow;
      throw OcrException('Failed to process image: $e');
    }
  }

  /// Extract text and analyze image in parallel
  /// Returns combined OCR and image analysis results
  Future<OcrResult> processImage(String imagePath) async {
    // Validate file exists
    final file = File(imagePath);
    if (!await file.exists()) {
      throw OcrException('Image file not found: $imagePath');
    }

    try {
      // Run OCR and image analysis in parallel
      final results = await Future.wait([
        _extractText(imagePath),
        _analyzeImage(imagePath),
      ]);

      final textLines = results[0] as List<String>;
      final imageAnalysis = results[1] as ImageAnalysisResult?;

      return OcrResult(
        textLines: textLines,
        imageAnalysis: imageAnalysis,
      );
    } catch (e) {
      if (e is OcrException) rethrow;
      throw OcrException('Failed to process image: $e');
    }
  }

  Future<List<String>> _extractText(String imagePath) async {
    final inputImage = InputImage.fromFilePath(imagePath);
    final recognizedText = await _recognizer.processImage(inputImage);

    return recognizedText.blocks
        .expand((block) => block.lines)
        .map((line) => line.text)
        .toList();
  }

  Future<ImageAnalysisResult?> _analyzeImage(String imagePath) async {
    try {
      return await _imageAnalysisService.analyzeImage(imagePath);
    } catch (e) {
      // Image analysis is optional - don't fail OCR if it fails
      return null;
    }
  }

  /// Clean up resources
  void dispose() {
    _recognizer.close();
  }
}

/// Exception thrown when OCR processing fails
class OcrException implements Exception {
  final String message;
  OcrException(this.message);

  @override
  String toString() => 'OcrException: $message';
}
