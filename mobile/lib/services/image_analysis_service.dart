import 'dart:io';
import 'dart:math';
import 'package:image/image.dart' as img;

/// Result of image analysis for foil detection and condition assessment
class ImageAnalysisResult {
  final bool isFoilDetected;
  final double foilConfidence;
  final String suggestedCondition;
  final double edgeWhiteningScore;
  final Map<String, double> cornerScores;

  ImageAnalysisResult({
    required this.isFoilDetected,
    required this.foilConfidence,
    required this.suggestedCondition,
    required this.edgeWhiteningScore,
    required this.cornerScores,
  });

  Map<String, dynamic> toJson() => {
        'is_foil_detected': isFoilDetected,
        'foil_confidence': foilConfidence,
        'suggested_condition': suggestedCondition,
        'edge_whitening_score': edgeWhiteningScore,
        'corner_scores': cornerScores,
      };
}

/// Service for analyzing card images to detect foil and assess condition
class ImageAnalysisService {
  // Threshold for foil detection (inter-region brightness variance)
  static const double foilVarianceThreshold = 0.15;

  // Brightness threshold for detecting whitening (0-1 scale)
  static const double whiteningBrightnessThreshold = 0.86; // ~220/255

  // Region size as percentage of image dimensions
  static const double cornerSizePercent = 0.10;
  static const double edgeWidthPercent = 0.05;

  /// Analyze an image file for foil detection and condition assessment
  Future<ImageAnalysisResult> analyzeImage(String imagePath) async {
    final file = File(imagePath);
    if (!await file.exists()) {
      throw ImageAnalysisException('Image file not found: $imagePath');
    }

    final bytes = await file.readAsBytes();
    final image = img.decodeImage(bytes);
    if (image == null) {
      throw ImageAnalysisException('Failed to decode image');
    }

    // Run foil detection and condition assessment
    final foilResult = _detectFoil(image);
    final conditionResult = _assessCondition(image);

    return ImageAnalysisResult(
      isFoilDetected: foilResult.isDetected,
      foilConfidence: foilResult.confidence,
      suggestedCondition: conditionResult.condition,
      edgeWhiteningScore: conditionResult.overallScore,
      cornerScores: conditionResult.cornerScores,
    );
  }

  /// Detect foil by analyzing brightness variance across image regions
  /// Foil cards have higher inter-region variance due to reflective surface
  _FoilDetectionResult _detectFoil(img.Image image) {
    const gridSize = 4;
    final regionWidth = image.width ~/ gridSize;
    final regionHeight = image.height ~/ gridSize;

    final regionBrightnesses = <double>[];

    // Calculate average brightness for each region in a 4x4 grid
    for (var gy = 0; gy < gridSize; gy++) {
      for (var gx = 0; gx < gridSize; gx++) {
        final startX = gx * regionWidth;
        final startY = gy * regionHeight;
        final brightness = _calculateRegionBrightness(
          image,
          startX,
          startY,
          regionWidth,
          regionHeight,
        );
        regionBrightnesses.add(brightness);
      }
    }

    // Calculate variance of brightnesses across regions
    final mean =
        regionBrightnesses.reduce((a, b) => a + b) / regionBrightnesses.length;
    final variance = regionBrightnesses
            .map((b) => pow(b - mean, 2))
            .reduce((a, b) => a + b) /
        regionBrightnesses.length;

    // Normalize variance to 0-1 scale (assuming max reasonable variance of ~0.1)
    final normalizedVariance = (variance / 0.1).clamp(0.0, 1.0);

    final isDetected = normalizedVariance > foilVarianceThreshold;
    // Convert variance to confidence score
    final confidence = isDetected
        ? ((normalizedVariance - foilVarianceThreshold) /
                (1.0 - foilVarianceThreshold))
            .clamp(0.0, 1.0)
        : 0.0;

    return _FoilDetectionResult(isDetected: isDetected, confidence: confidence);
  }

  /// Assess card condition by analyzing edge whitening
  _ConditionAssessmentResult _assessCondition(img.Image image) {
    final cornerSize = (min(image.width, image.height) * cornerSizePercent).round();
    final edgeWidth = (min(image.width, image.height) * edgeWidthPercent).round();

    // Extract corner regions and calculate whitening scores
    final cornerScores = <String, double>{};

    // Top-left corner
    cornerScores['topLeft'] = _calculateWhiteningScore(
      image,
      0,
      0,
      cornerSize,
      cornerSize,
    );

    // Top-right corner
    cornerScores['topRight'] = _calculateWhiteningScore(
      image,
      image.width - cornerSize,
      0,
      cornerSize,
      cornerSize,
    );

    // Bottom-left corner
    cornerScores['bottomLeft'] = _calculateWhiteningScore(
      image,
      0,
      image.height - cornerSize,
      cornerSize,
      cornerSize,
    );

    // Bottom-right corner
    cornerScores['bottomRight'] = _calculateWhiteningScore(
      image,
      image.width - cornerSize,
      image.height - cornerSize,
      cornerSize,
      cornerSize,
    );

    // Extract edge strips (excluding corners)
    final edgeScores = <double>[];

    // Top edge
    edgeScores.add(_calculateWhiteningScore(
      image,
      cornerSize,
      0,
      image.width - 2 * cornerSize,
      edgeWidth,
    ));

    // Bottom edge
    edgeScores.add(_calculateWhiteningScore(
      image,
      cornerSize,
      image.height - edgeWidth,
      image.width - 2 * cornerSize,
      edgeWidth,
    ));

    // Left edge
    edgeScores.add(_calculateWhiteningScore(
      image,
      0,
      cornerSize,
      edgeWidth,
      image.height - 2 * cornerSize,
    ));

    // Right edge
    edgeScores.add(_calculateWhiteningScore(
      image,
      image.width - edgeWidth,
      cornerSize,
      edgeWidth,
      image.height - 2 * cornerSize,
    ));

    // Calculate overall score (weighted: corners are more important)
    final cornerAverage =
        cornerScores.values.reduce((a, b) => a + b) / cornerScores.length;
    final edgeAverage = edgeScores.reduce((a, b) => a + b) / edgeScores.length;
    final overallScore = cornerAverage * 0.7 + edgeAverage * 0.3;

    // Map score to condition grade
    final condition = _mapScoreToCondition(overallScore);

    return _ConditionAssessmentResult(
      condition: condition,
      overallScore: overallScore,
      cornerScores: cornerScores,
    );
  }

  /// Calculate average brightness for a region (0-1 scale)
  double _calculateRegionBrightness(
    img.Image image,
    int startX,
    int startY,
    int width,
    int height,
  ) {
    var totalBrightness = 0.0;
    var pixelCount = 0;

    // Sample every 4th pixel for performance
    for (var y = startY; y < startY + height && y < image.height; y += 4) {
      for (var x = startX; x < startX + width && x < image.width; x += 4) {
        final pixel = image.getPixel(x, y);
        // Calculate perceived brightness using luminance formula
        final brightness =
            (0.299 * pixel.r + 0.587 * pixel.g + 0.114 * pixel.b) / 255.0;
        totalBrightness += brightness;
        pixelCount++;
      }
    }

    return pixelCount > 0 ? totalBrightness / pixelCount : 0.0;
  }

  /// Calculate whitening score for a region (0-1, higher = more whitening)
  double _calculateWhiteningScore(
    img.Image image,
    int startX,
    int startY,
    int width,
    int height,
  ) {
    if (width <= 0 || height <= 0) return 0.0;

    var brightPixelCount = 0;
    var totalPixelCount = 0;

    // Sample every 2nd pixel for better accuracy in small regions
    for (var y = startY; y < startY + height && y < image.height; y += 2) {
      for (var x = startX; x < startX + width && x < image.width; x += 2) {
        final pixel = image.getPixel(x, y);
        final brightness =
            (0.299 * pixel.r + 0.587 * pixel.g + 0.114 * pixel.b) / 255.0;

        if (brightness > whiteningBrightnessThreshold) {
          brightPixelCount++;
        }
        totalPixelCount++;
      }
    }

    return totalPixelCount > 0 ? brightPixelCount / totalPixelCount : 0.0;
  }

  /// Map whitening score to condition grade
  String _mapScoreToCondition(double score) {
    // Lower score = less whitening = better condition
    if (score < 0.05) return 'NM'; // Near Mint
    if (score < 0.15) return 'LP'; // Lightly Played
    if (score < 0.30) return 'MP'; // Moderately Played
    return 'HP'; // Heavily Played
  }
}

class _FoilDetectionResult {
  final bool isDetected;
  final double confidence;

  _FoilDetectionResult({
    required this.isDetected,
    required this.confidence,
  });
}

class _ConditionAssessmentResult {
  final String condition;
  final double overallScore;
  final Map<String, double> cornerScores;

  _ConditionAssessmentResult({
    required this.condition,
    required this.overallScore,
    required this.cornerScores,
  });
}

/// Exception thrown when image analysis fails
class ImageAnalysisException implements Exception {
  final String message;
  ImageAnalysisException(this.message);

  @override
  String toString() => 'ImageAnalysisException: $message';
}
