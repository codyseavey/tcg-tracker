import 'package:flutter/material.dart';
import '../models/card.dart';
import '../services/api_service.dart';

class ScanResultScreen extends StatefulWidget {
  final List<CardModel> cards;
  final String searchQuery;
  final ScanMetadata? scanMetadata;
  final ApiService? apiService;

  const ScanResultScreen({
    super.key,
    required this.cards,
    required this.searchQuery,
    this.scanMetadata,
    this.apiService,
  });

  @override
  State<ScanResultScreen> createState() => _ScanResultScreenState();
}

class _ScanResultScreenState extends State<ScanResultScreen> {
  late final ApiService _apiService;
  int _quantity = 1;
  late String _condition;
  late bool _foil;
  bool _isAdding = false;

  // Condition codes: M=Mint, NM=Near Mint, LP=Lightly Played, MP=Moderately Played, HP=Heavily Played, D=Damaged
  final List<String> _conditions = ['M', 'NM', 'LP', 'MP', 'HP', 'D'];

  @override
  void initState() {
    super.initState();
    _apiService = widget.apiService ?? ApiService();
    // Pre-fill foil based on scan detection (text or image analysis)
    _foil = widget.scanMetadata?.isFoil ?? false;
    // Pre-fill condition based on image analysis suggested condition
    final suggested = widget.scanMetadata?.suggestedCondition;
    _condition = (suggested != null && _conditions.contains(suggested)) ? suggested : 'NM';
  }

  Future<void> _addToCollection(CardModel card) async {
    setState(() => _isAdding = true);

    try {
      await _apiService.addToCollection(
        card.id,
        quantity: _quantity,
        condition: _condition,
        foil: _foil,
      );

      if (!mounted) return;

      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text('Added ${card.name} to collection!'),
          backgroundColor: Colors.green,
        ),
      );

      Navigator.pop(context);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Error: ${e.toString()}'),
            backgroundColor: Colors.red,
          ),
        );
      }
    } finally {
      if (mounted) {
        setState(() => _isAdding = false);
      }
    }
  }

  void _showAddDialog(CardModel card) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (context) => StatefulBuilder(
        builder: (context, setModalState) => Padding(
          padding: EdgeInsets.only(
            bottom: MediaQuery.of(context).viewInsets.bottom,
            left: 16,
            right: 16,
            top: 16,
          ),
          // Wrap in SingleChildScrollView to handle overflow on small screens
          child: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  'Add ${card.name}',
                  style: Theme.of(context).textTheme.titleLarge,
                ),
                const SizedBox(height: 16),
                // Quantity
                Row(
                  children: [
                    const Text('Quantity:'),
                    const Spacer(),
                    IconButton(
                      icon: const Icon(Icons.remove),
                      onPressed: _quantity > 1
                          ? () => setModalState(() => _quantity--)
                          : null,
                    ),
                    Text('$_quantity', style: const TextStyle(fontSize: 18)),
                    IconButton(
                      icon: const Icon(Icons.add),
                      onPressed: () => setModalState(() => _quantity++),
                    ),
                  ],
                ),
                // Condition with auto-detect indicator
                Row(
                  children: [
                    const Text('Condition:'),
                    if (widget.scanMetadata?.suggestedCondition != null) ...[
                      const SizedBox(width: 8),
                      Container(
                        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                        decoration: BoxDecoration(
                          color: _getConditionColor(_condition),
                          borderRadius: BorderRadius.circular(8),
                        ),
                        child: const Text(
                          'Auto',
                          style: TextStyle(fontSize: 10, color: Colors.white),
                        ),
                      ),
                    ],
                    const SizedBox(width: 8),
                    Expanded(
                      child: DropdownButton<String>(
                        value: _condition,
                        isExpanded: true,
                        items: _conditions.map((c) {
                          return DropdownMenuItem(
                            value: c,
                            child: Text('$c - ${_getConditionDescription(c)}'),
                          );
                        }).toList(),
                        onChanged: (value) {
                          if (value != null) {
                            setModalState(() => _condition = value);
                          }
                        },
                      ),
                    ),
                  ],
                ),
                // Foil with detection indicator
                SwitchListTile(
                  title: Row(
                    children: [
                      const Text('Foil'),
                      if (widget.scanMetadata?.isFoil == true) ...[
                        const SizedBox(width: 8),
                        Container(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 8,
                            vertical: 2,
                          ),
                          decoration: BoxDecoration(
                            color: Theme.of(context).colorScheme.tertiaryContainer,
                            borderRadius: BorderRadius.circular(12),
                          ),
                          child: Text(
                            'Detected',
                            style: TextStyle(
                              fontSize: 12,
                              color: Theme.of(context).colorScheme.onTertiaryContainer,
                            ),
                          ),
                        ),
                      ],
                    ],
                  ),
                  value: _foil,
                  onChanged: (value) => setModalState(() => _foil = value),
                ),
                const SizedBox(height: 16),
                FilledButton(
                  onPressed: _isAdding ? null : () {
                    Navigator.pop(context);
                    _addToCollection(card);
                  },
                  child: _isAdding
                      ? const SizedBox(
                          height: 20,
                          width: 20,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Text('Add to Collection'),
                ),
                const SizedBox(height: 16),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildScanInfoCard() {
    final meta = widget.scanMetadata;
    if (meta == null || meta.confidence == 0) {
      return const SizedBox.shrink();
    }

    return Card(
      margin: const EdgeInsets.all(8),
      color: Theme.of(context).colorScheme.primaryContainer,
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  Icons.document_scanner,
                  size: 20,
                  color: Theme.of(context).colorScheme.onPrimaryContainer,
                ),
                const SizedBox(width: 8),
                Text(
                  'Scan Detection',
                  style: TextStyle(
                    fontWeight: FontWeight.bold,
                    color: Theme.of(context).colorScheme.onPrimaryContainer,
                  ),
                ),
                const Spacer(),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                  decoration: BoxDecoration(
                    color: _getConfidenceColor(context, meta.confidence),
                    borderRadius: BorderRadius.circular(12),
                  ),
                  child: Text(
                    '${(meta.confidence * 100).toInt()}% confidence',
                    style: TextStyle(
                      fontSize: 12,
                      color: Theme.of(context).colorScheme.onPrimary,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              meta.detectionSummary,
              style: TextStyle(
                color: Theme.of(context).colorScheme.onPrimaryContainer,
              ),
            ),
            // Condition assessment display
            if (meta.suggestedCondition != null) ...[
              const SizedBox(height: 8),
              _buildConditionIndicator(meta),
            ],
            // Foil confidence display
            if (meta.foilConfidence != null && meta.foilConfidence! > 0) ...[
              const SizedBox(height: 8),
              _buildFoilConfidenceIndicator(meta),
            ],
            // Corner scores visualization
            if (meta.cornerScores != null && meta.cornerScores!.isNotEmpty) ...[
              const SizedBox(height: 8),
              _buildCornerScoresGrid(meta.cornerScores!),
            ],
            if (meta.foilIndicators.isNotEmpty) ...[
              const SizedBox(height: 4),
              Wrap(
                spacing: 4,
                children: meta.foilIndicators.map((indicator) {
                  return Chip(
                    label: Text(indicator, style: const TextStyle(fontSize: 10)),
                    visualDensity: VisualDensity.compact,
                    backgroundColor: Colors.amber.shade100,
                    padding: EdgeInsets.zero,
                  );
                }).toList(),
              ),
            ],
            if (meta.conditionHints.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                'Condition hints: ${meta.conditionHints.join(", ")}',
                style: TextStyle(
                  fontSize: 12,
                  fontStyle: FontStyle.italic,
                  color: Theme.of(context).colorScheme.onPrimaryContainer.withValues(alpha: 0.7),
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }

  Widget _buildConditionIndicator(ScanMetadata meta) {
    final condition = meta.suggestedCondition!;
    final color = _getConditionColor(condition);
    final description = _getConditionDescription(condition);

    return Row(
      children: [
        Icon(Icons.verified, size: 16, color: color),
        const SizedBox(width: 4),
        Text(
          'Suggested Condition: ',
          style: TextStyle(
            fontSize: 12,
            color: Theme.of(context).colorScheme.onPrimaryContainer,
          ),
        ),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
          decoration: BoxDecoration(
            color: color,
            borderRadius: BorderRadius.circular(8),
          ),
          child: Text(
            condition,
            style: const TextStyle(
              fontSize: 12,
              color: Colors.white,
              fontWeight: FontWeight.bold,
            ),
          ),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: Text(
            description,
            style: TextStyle(
              fontSize: 11,
              color: Theme.of(context).colorScheme.onPrimaryContainer.withValues(alpha: 0.7),
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildFoilConfidenceIndicator(ScanMetadata meta) {
    final confidence = meta.foilConfidence!;
    final isHighConfidence = confidence >= 0.7;

    return Row(
      children: [
        Icon(
          Icons.auto_awesome,
          size: 16,
          color: isHighConfidence ? Colors.amber : Colors.grey,
        ),
        const SizedBox(width: 4),
        Text(
          'Foil Detection: ',
          style: TextStyle(
            fontSize: 12,
            color: Theme.of(context).colorScheme.onPrimaryContainer,
          ),
        ),
        Container(
          width: 60,
          height: 8,
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(4),
            color: Colors.grey.shade300,
          ),
          child: FractionallySizedBox(
            alignment: Alignment.centerLeft,
            widthFactor: confidence,
            child: Container(
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(4),
                color: isHighConfidence ? Colors.amber : Colors.grey,
              ),
            ),
          ),
        ),
        const SizedBox(width: 4),
        Text(
          '${(confidence * 100).toInt()}%',
          style: TextStyle(
            fontSize: 11,
            color: Theme.of(context).colorScheme.onPrimaryContainer,
          ),
        ),
      ],
    );
  }

  Widget _buildCornerScoresGrid(Map<String, double> cornerScores) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'Edge Whitening Detection:',
          style: TextStyle(
            fontSize: 12,
            color: Theme.of(context).colorScheme.onPrimaryContainer,
          ),
        ),
        const SizedBox(height: 4),
        SizedBox(
          width: 80,
          height: 80,
          child: CustomPaint(
            painter: CornerScoresPainter(cornerScores),
          ),
        ),
      ],
    );
  }

  Color _getConditionColor(String condition) {
    switch (condition) {
      case 'M':
        return Colors.blue;
      case 'NM':
        return Colors.green;
      case 'LP':
        return Colors.lightGreen;
      case 'MP':
        return Colors.orange;
      case 'HP':
        return Colors.deepOrange;
      case 'D':
        return Colors.red;
      default:
        return Colors.grey;
    }
  }

  String _getConditionDescription(String condition) {
    switch (condition) {
      case 'M':
        return 'Mint';
      case 'NM':
        return 'Near Mint';
      case 'LP':
        return 'Lightly Played';
      case 'MP':
        return 'Moderately Played';
      case 'HP':
        return 'Heavily Played';
      case 'D':
        return 'Damaged';
      default:
        return condition;
    }
  }

  Color _getConfidenceColor(BuildContext context, double confidence) {
    final colorScheme = Theme.of(context).colorScheme;
    if (confidence >= 0.7) return colorScheme.primary;
    if (confidence >= 0.4) return colorScheme.tertiary;
    return colorScheme.error;
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text('Results for "${widget.searchQuery}"'),
        backgroundColor: Theme.of(context).colorScheme.inversePrimary,
      ),
      body: widget.cards.isEmpty
          ? const Center(child: Text('No cards found'))
          : Column(
              children: [
                _buildScanInfoCard(),
                Expanded(
                  child: ListView.builder(
                    itemCount: widget.cards.length,
                    itemBuilder: (context, index) {
                      final card = widget.cards[index];
                      return Card(
                        margin: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                        child: ListTile(
                          leading: card.imageUrl != null
                              ? ClipRRect(
                                  borderRadius: BorderRadius.circular(4),
                                  child: SizedBox(
                                    width: MediaQuery.of(context).size.width * 0.12,
                                    child: AspectRatio(
                                      aspectRatio: 2.5 / 3.5,
                                      child: Image.network(
                                        card.imageUrl!,
                                        fit: BoxFit.cover,
                                        errorBuilder: (context, error, stackTrace) => const Icon(Icons.image),
                                      ),
                                    ),
                                  ),
                                )
                              : const Icon(Icons.image),
                          title: Text(card.name),
                          subtitle: Text('${card.displaySet} • ${card.displayPrice}'),
                          trailing: IconButton(
                            icon: const Icon(Icons.add_circle),
                            color: Theme.of(context).colorScheme.primary,
                            onPressed: () => _showAddDialog(card),
                          ),
                          onTap: () => _showAddDialog(card),
                        ),
                      );
                    },
                  ),
                ),
              ],
            ),
    );
  }
}

/// Custom painter for visualizing corner whitening scores
class CornerScoresPainter extends CustomPainter {
  final Map<String, double> cornerScores;

  CornerScoresPainter(this.cornerScores);

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()..style = PaintingStyle.fill;
    final borderPaint = Paint()
      ..style = PaintingStyle.stroke
      ..color = Colors.grey
      ..strokeWidth = 1;

    // Draw card outline
    final cardRect = Rect.fromLTWH(0, 0, size.width, size.height);
    canvas.drawRect(cardRect, borderPaint);

    final cornerSize = size.width * 0.25;

    // Draw corners with color based on whitening score
    _drawCorner(canvas, paint, 0, 0, cornerSize, cornerScores['topLeft'] ?? 0);
    _drawCorner(
        canvas, paint, size.width - cornerSize, 0, cornerSize, cornerScores['topRight'] ?? 0);
    _drawCorner(canvas, paint, 0, size.height - cornerSize, cornerSize,
        cornerScores['bottomLeft'] ?? 0);
    _drawCorner(canvas, paint, size.width - cornerSize, size.height - cornerSize, cornerSize,
        cornerScores['bottomRight'] ?? 0);
  }

  void _drawCorner(Canvas canvas, Paint paint, double x, double y, double size, double score) {
    // Green = good (low whitening), Red = bad (high whitening)
    paint.color = Color.lerp(Colors.green, Colors.red, score) ?? Colors.grey;
    canvas.drawRect(Rect.fromLTWH(x, y, size, size), paint);
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => true;
}
